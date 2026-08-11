package prompttemplate

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	CodeInvalid = "PROMPT_TEMPLATE_INVALID"
	CodeStale   = "PROMPT_TEMPLATE_STALE"
)

type Kind string

const (
	KindText      Kind = "text"
	KindReference Kind = "reference"
	KindVariable  Kind = "variable"
)

type Limits struct {
	MaxTemplateRunes int
	MaxExpandedRunes int
	MaxNameRunes     int
	MaxOccurrences   int
	MaxVariables     int
}

func DefaultLimits() Limits {
	return Limits{MaxTemplateRunes: 4000, MaxExpandedRunes: 4000, MaxNameRunes: 64, MaxOccurrences: 100, MaxVariables: 50}
}

type Error struct {
	Code    string
	Field   string
	Name    string
	Offset  int
	Rule    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type Occurrence struct {
	Kind  Kind
	Name  string
	Start int
	End   int
}

type Segment struct {
	Kind   Kind
	Text   string
	Source string
	Name   string
	Start  int
	End    int
}

type Document struct {
	Template       string
	Canonical      string
	Segments       []Segment
	Occurrences    []Occurrence
	ReferenceNames []string
	VariableNames  []string
}

func Parse(template string, limits Limits) (Document, error) {
	limits = normalizeLimits(limits)
	runes := []rune(template)
	if len(runes) > limits.MaxTemplateRunes {
		return Document{}, invalidError("prompt", "", limits.MaxTemplateRunes, "template_length", "提示词模板超过长度限制")
	}

	document := Document{Template: template}
	references := map[string]struct{}{}
	variables := map[string]struct{}{}
	canonical := strings.Builder{}
	textStart := 0
	flushText := func(end int) {
		if end <= textStart {
			return
		}
		source := string(runes[textStart:end])
		document.Segments = appendTextSegment(document.Segments, Segment{Kind: KindText, Text: source, Source: source, Start: textStart, End: end})
		canonical.WriteString(source)
	}

	for index := 0; index < len(runes); {
		if runes[index] == '\\' && index+1 < len(runes) {
			kind, prefix := placeholderPrefix(runes, index+1)
			if kind != "" {
				closeAt := findClosing(runes, index+1+len(prefix))
				if closeAt >= 0 {
					flushText(index)
					end := closeAt + 2
					source := string(runes[index:end])
					literal := string(runes[index+1 : end])
					document.Segments = appendTextSegment(document.Segments, Segment{Kind: KindText, Text: literal, Source: source, Start: index, End: end})
					canonical.WriteString(source)
					index, textStart = end, end
					continue
				}
			}
		}

		kind, prefix := placeholderPrefix(runes, index)
		if kind == "" {
			index++
			continue
		}
		flushText(index)
		nameStart := index + len(prefix)
		closeAt := findClosing(runes, nameStart)
		if closeAt < 0 {
			return Document{}, invalidError("prompt", "", index, "unclosed", "提示词占位符未闭合")
		}
		rawName := runes[nameStart:closeAt]
		if containsNested(rawName) {
			return Document{}, invalidError("prompt", "", index, "nested", "提示词占位符不能嵌套")
		}
		name, err := normalizeNameRunes(rawName, limits.MaxNameRunes)
		if err != nil {
			err.Offset = index
			return Document{}, err
		}
		end := closeAt + 2
		document.Segments = append(document.Segments, Segment{Kind: kind, Name: name, Source: string(runes[index:end]), Start: index, End: end})
		document.Occurrences = append(document.Occurrences, Occurrence{Kind: kind, Name: name, Start: index, End: end})
		if len(document.Occurrences) > limits.MaxOccurrences {
			return Document{}, invalidError("prompt", name, index, "occurrence_limit", "提示词占位符数量超过限制")
		}
		if kind == KindReference {
			if _, exists := references[name]; !exists {
				references[name] = struct{}{}
				document.ReferenceNames = append(document.ReferenceNames, name)
			}
			canonical.WriteString("{{@" + name + "}}")
		} else {
			if _, exists := variables[name]; !exists {
				if len(variables) >= limits.MaxVariables {
					return Document{}, invalidError("prompt", name, index, "variable_limit", "提示词变量数量超过限制")
				}
				variables[name] = struct{}{}
				document.VariableNames = append(document.VariableNames, name)
			}
			canonical.WriteString("{{$" + name + "}}")
		}
		index, textStart = end, end
	}
	flushText(len(runes))
	document.Canonical = canonical.String()
	return document, nil
}

func NormalizeName(raw string, maxRunes int) (string, error) {
	name, err := normalizeNameRunes([]rune(raw), maxRunes)
	if err != nil {
		return "", err
	}
	return name, nil
}

func normalizeNameRunes(raw []rune, maxRunes int) (string, *Error) {
	name := norm.NFC.String(strings.TrimSpace(string(raw)))
	if name == "" {
		return "", invalidError("name", "", 0, "name_empty", "占位符名称不能为空")
	}
	if len([]rune(name)) > maxRunes {
		return "", invalidError("name", name, 0, "name_length", fmt.Sprintf("占位符名称不能超过 %d 个字符", maxRunes))
	}
	for _, value := range name {
		if value == '{' || value == '}' || value == '\n' || value == '\r' || value == '\t' || unicode.IsControl(value) {
			return "", invalidError("name", name, 0, "name_character", "占位符名称包含不允许的字符")
		}
	}
	return name, nil
}

func normalizeLimits(limits Limits) Limits {
	defaults := DefaultLimits()
	if limits.MaxTemplateRunes <= 0 {
		limits.MaxTemplateRunes = defaults.MaxTemplateRunes
	}
	if limits.MaxExpandedRunes <= 0 {
		limits.MaxExpandedRunes = defaults.MaxExpandedRunes
	}
	if limits.MaxNameRunes <= 0 {
		limits.MaxNameRunes = defaults.MaxNameRunes
	}
	if limits.MaxOccurrences <= 0 {
		limits.MaxOccurrences = defaults.MaxOccurrences
	}
	if limits.MaxVariables <= 0 {
		limits.MaxVariables = defaults.MaxVariables
	}
	return limits
}

func placeholderPrefix(runes []rune, index int) (Kind, []rune) {
	if index+3 > len(runes) || runes[index] != '{' || runes[index+1] != '{' {
		return "", nil
	}
	switch runes[index+2] {
	case '@':
		return KindReference, []rune("{{@")
	case '$':
		return KindVariable, []rune("{{$")
	default:
		return "", nil
	}
}

func findClosing(runes []rune, start int) int {
	for index := start; index+1 < len(runes); index++ {
		if runes[index] == '}' && runes[index+1] == '}' {
			return index
		}
	}
	return -1
}

func containsNested(runes []rune) bool {
	for index, value := range runes {
		if value == '{' || value == '}' {
			return true
		}
		if index+2 < len(runes) && runes[index] == '{' && runes[index+1] == '{' {
			return true
		}
	}
	return false
}

func appendTextSegment(segments []Segment, segment Segment) []Segment {
	if len(segments) > 0 && segments[len(segments)-1].Kind == KindText && segments[len(segments)-1].End == segment.Start {
		segments[len(segments)-1].Text += segment.Text
		segments[len(segments)-1].Source += segment.Source
		segments[len(segments)-1].End = segment.End
		return segments
	}
	return append(segments, segment)
}

func invalidError(field, name string, offset int, rule, message string) *Error {
	return &Error{Code: CodeInvalid, Field: field, Name: name, Offset: offset, Rule: rule, Message: message}
}
