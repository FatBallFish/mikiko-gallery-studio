package prompttemplate

import (
	"fmt"
	"strings"
)

type ReferenceBinding struct {
	Name    string `json:"name"`
	AssetID string `json:"asset_id"`
}

type VariableBinding struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Asset struct {
	ID   string
	Name string
}

type SnapshotReference struct {
	Name    string
	AssetID string
	Index   int
}

type Snapshot struct {
	References    []SnapshotReference
	VariableNames []string
}

type ResolveRequest struct {
	Template          string
	ReferenceAssetIDs []string
	ReferenceBindings []ReferenceBinding
	VariableBindings  []VariableBinding
	Assets            []Asset
	Limits            Limits
}

type ResolveResult struct {
	CanonicalTemplate string
	Expanded          string
	Snapshot          Snapshot
}

func Resolve(request ResolveRequest) (ResolveResult, error) {
	limits := normalizeLimits(request.Limits)
	document, err := Parse(request.Template, limits)
	if err != nil {
		return ResolveResult{}, err
	}
	selected, selectedIndex, err := normalizeSelectedIDs(request.ReferenceAssetIDs)
	if err != nil {
		return ResolveResult{}, err
	}
	assets := make(map[string]Asset, len(request.Assets))
	for _, asset := range request.Assets {
		id := strings.TrimSpace(asset.ID)
		if id == "" {
			continue
		}
		name, nameErr := NormalizeName(asset.Name, limits.MaxNameRunes)
		if nameErr != nil {
			return ResolveResult{}, invalidError("reference_bindings", asset.Name, 0, "reference_asset_name_invalid", "引用资产名称不合法")
		}
		assets[id] = Asset{ID: id, Name: name}
	}

	references, err := normalizeReferenceBindings(request.ReferenceBindings, limits.MaxNameRunes)
	if err != nil {
		return ResolveResult{}, err
	}
	variables, err := normalizeVariableBindings(request.VariableBindings, limits)
	if err != nil {
		return ResolveResult{}, err
	}
	requiredReferences := stringSet(document.ReferenceNames)
	requiredVariables := stringSet(document.VariableNames)
	for name := range references {
		if _, required := requiredReferences[name]; !required {
			return ResolveResult{}, invalidError("reference_bindings", name, 0, "reference_extra", "提交了模板未使用的资源绑定")
		}
	}
	for name := range variables {
		if _, required := requiredVariables[name]; !required {
			return ResolveResult{}, invalidError("prompt_variables", name, 0, "variable_extra", "提交了模板未使用的变量")
		}
	}

	result := ResolveResult{CanonicalTemplate: document.Canonical}
	for _, name := range document.ReferenceNames {
		binding, exists := references[name]
		if !exists {
			return ResolveResult{}, invalidError("reference_bindings", name, occurrenceOffset(document, KindReference, name), "reference_missing", "资源占位符尚未绑定引用资产")
		}
		index, selected := selectedIndex[binding.AssetID]
		if !selected {
			return ResolveResult{}, invalidError("reference_bindings", name, occurrenceOffset(document, KindReference, name), "reference_not_selected", "资源绑定未包含在已选引用资产中")
		}
		asset, exists := assets[binding.AssetID]
		if !exists {
			return ResolveResult{}, invalidError("reference_bindings", name, occurrenceOffset(document, KindReference, name), "reference_asset_missing", "引用资产不存在或不可用")
		}
		if asset.Name != name {
			return ResolveResult{}, &Error{Code: CodeStale, Field: "reference_bindings", Name: name, Offset: occurrenceOffset(document, KindReference, name), Rule: "reference_name_changed", Message: "引用资产名称已变化，请刷新后确认"}
		}
		result.Snapshot.References = append(result.Snapshot.References, SnapshotReference{Name: name, AssetID: binding.AssetID, Index: index + 1})
	}
	for _, name := range document.VariableNames {
		if _, exists := variables[name]; !exists {
			return ResolveResult{}, invalidError("prompt_variables", name, occurrenceOffset(document, KindVariable, name), "variable_missing", "模板变量尚未填写")
		}
		result.Snapshot.VariableNames = append(result.Snapshot.VariableNames, name)
	}

	expanded := strings.Builder{}
	for _, segment := range document.Segments {
		switch segment.Kind {
		case KindText:
			expanded.WriteString(segment.Text)
		case KindReference:
			expanded.WriteString(fmt.Sprintf("图片%d", selectedIndex[references[segment.Name].AssetID]+1))
		case KindVariable:
			expanded.WriteString(variables[segment.Name])
		}
	}
	if len([]rune(expanded.String())) > limits.MaxExpandedRunes {
		return ResolveResult{}, invalidError("prompt", "", limits.MaxExpandedRunes, "expanded_length", "变量填充后的提示词超过长度限制")
	}
	_ = selected
	result.Expanded = expanded.String()
	return result, nil
}

func normalizeSelectedIDs(raw []string) ([]string, map[string]int, error) {
	selected := make([]string, 0, len(raw))
	indexes := make(map[string]int, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == "" {
			return nil, nil, invalidError("reference_asset_ids", "", 0, "reference_id_empty", "引用资产 ID 不能为空")
		}
		if _, exists := indexes[id]; exists {
			return nil, nil, invalidError("reference_asset_ids", "", 0, "reference_id_duplicate", "引用资产不能重复选择")
		}
		indexes[id] = len(selected)
		selected = append(selected, id)
	}
	return selected, indexes, nil
}

func normalizeReferenceBindings(raw []ReferenceBinding, maxNameRunes int) (map[string]ReferenceBinding, error) {
	result := make(map[string]ReferenceBinding, len(raw))
	for _, binding := range raw {
		name, err := NormalizeName(binding.Name, maxNameRunes)
		if err != nil {
			return nil, invalidError("reference_bindings", binding.Name, 0, err.(*Error).Rule, err.Error())
		}
		if _, exists := result[name]; exists {
			return nil, invalidError("reference_bindings", name, 0, "reference_duplicate", "资源名称存在重复绑定")
		}
		binding.Name, binding.AssetID = name, strings.TrimSpace(binding.AssetID)
		result[name] = binding
	}
	return result, nil
}

func normalizeVariableBindings(raw []VariableBinding, limits Limits) (map[string]string, error) {
	result := make(map[string]string, len(raw))
	for _, binding := range raw {
		name, err := NormalizeName(binding.Name, limits.MaxNameRunes)
		if err != nil {
			return nil, invalidError("prompt_variables", binding.Name, 0, err.(*Error).Rule, err.Error())
		}
		if _, exists := result[name]; exists {
			return nil, invalidError("prompt_variables", name, 0, "variable_duplicate", "变量名称存在重复绑定")
		}
		value := strings.TrimSpace(binding.Value)
		if value == "" {
			return nil, invalidError("prompt_variables", name, 0, "variable_blank", "模板变量尚未填写")
		}
		if len([]rune(value)) > limits.MaxExpandedRunes {
			return nil, invalidError("prompt_variables", name, 0, "variable_length", "变量值超过长度限制")
		}
		result[name] = value
	}
	return result, nil
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func occurrenceOffset(document Document, kind Kind, name string) int {
	for _, occurrence := range document.Occurrences {
		if occurrence.Kind == kind && occurrence.Name == name {
			return occurrence.Start
		}
	}
	return 0
}
