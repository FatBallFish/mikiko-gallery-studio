package prompttemplate_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/domain/prompttemplate"
)

type parserFixtures struct {
	Valid []struct {
		Name        string   `json:"name"`
		Template    string   `json:"template"`
		Canonical   string   `json:"canonical"`
		References  []string `json:"references"`
		Variables   []string `json:"variables"`
		Occurrences []struct {
			Kind  string `json:"kind"`
			Name  string `json:"name"`
			Start int    `json:"start"`
			End   int    `json:"end"`
		} `json:"occurrences"`
	} `json:"valid"`
	Invalid []struct {
		Name     string `json:"name"`
		Template string `json:"template"`
		Rule     string `json:"rule"`
		Offset   int    `json:"offset"`
	} `json:"invalid"`
}

func TestParseSharedFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	for _, fixture := range fixtures.Valid {
		t.Run(fixture.Name, func(t *testing.T) {
			document, err := prompttemplate.Parse(fixture.Template, prompttemplate.DefaultLimits())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if document.Canonical != fixture.Canonical {
				t.Fatalf("canonical = %q, want %q", document.Canonical, fixture.Canonical)
			}
			assertStrings(t, document.ReferenceNames, fixture.References)
			assertStrings(t, document.VariableNames, fixture.Variables)
			if len(document.Occurrences) != len(fixture.Occurrences) {
				t.Fatalf("occurrences = %#v, want %#v", document.Occurrences, fixture.Occurrences)
			}
			for index, want := range fixture.Occurrences {
				got := document.Occurrences[index]
				if string(got.Kind) != want.Kind || got.Name != want.Name || got.Start != want.Start || got.End != want.End {
					t.Fatalf("occurrence %d = %#v, want %#v", index, got, want)
				}
			}
		})
	}
}

func TestParseRejectsInvalidSharedFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	for _, fixture := range fixtures.Invalid {
		t.Run(fixture.Name, func(t *testing.T) {
			_, err := prompttemplate.Parse(fixture.Template, prompttemplate.DefaultLimits())
			parseErr, ok := err.(*prompttemplate.Error)
			if !ok {
				t.Fatalf("error = %T %v, want *prompttemplate.Error", err, err)
			}
			if parseErr.Rule != fixture.Rule || parseErr.Offset != fixture.Offset {
				t.Fatalf("error = %#v, want rule=%q offset=%d", parseErr, fixture.Rule, fixture.Offset)
			}
		})
	}
}

func TestParseEnforcesTemplateTokenAndNameLimits(t *testing.T) {
	tests := []struct {
		name     string
		template string
		limits   prompttemplate.Limits
		rule     string
	}{
		{name: "template", template: "12345", limits: prompttemplate.Limits{MaxTemplateRunes: 4, MaxNameRunes: 64, MaxOccurrences: 100}, rule: "template_length"},
		{name: "name", template: "{{@12345}}", limits: prompttemplate.Limits{MaxTemplateRunes: 100, MaxNameRunes: 4, MaxOccurrences: 100}, rule: "name_length"},
		{name: "occurrences", template: "{{@a}}{{@b}}", limits: prompttemplate.Limits{MaxTemplateRunes: 100, MaxNameRunes: 64, MaxOccurrences: 1}, rule: "occurrence_limit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := prompttemplate.Parse(test.template, test.limits)
			parseErr, ok := err.(*prompttemplate.Error)
			if !ok || parseErr.Rule != test.rule {
				t.Fatalf("error = %#v, want rule %q", err, test.rule)
			}
		})
	}
}

func TestParseTreatsNamespacesAndCaseAsDistinct(t *testing.T) {
	document, err := prompttemplate.Parse("{{@主体}}{{@主体}}{{@subject}}{{@Subject}}{{$主体}}", prompttemplate.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, document.ReferenceNames, []string{"主体", "subject", "Subject"})
	assertStrings(t, document.VariableNames, []string{"主体"})
}

func TestParseRejectsMoreThanFiftyDistinctVariables(t *testing.T) {
	var template strings.Builder
	for index := 1; index <= 51; index++ {
		template.WriteString(fmt.Sprintf("{{$变量%d}}", index))
	}
	_, err := prompttemplate.Parse(template.String(), prompttemplate.DefaultLimits())
	parseErr, ok := err.(*prompttemplate.Error)
	if !ok || parseErr.Rule != "variable_limit" {
		t.Fatalf("error = %#v, want variable_limit", err)
	}
}

func loadFixtures(t *testing.T) parserFixtures {
	t.Helper()
	_, filename, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(filename), "../../../testdata/prompt-template-fixtures.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures parserFixtures
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
