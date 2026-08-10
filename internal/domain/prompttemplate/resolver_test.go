package prompttemplate_test

import (
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/domain/prompttemplate"
)

func TestResolveExpandsVariablesAndReferenceIndexesDeterministically(t *testing.T) {
	result, err := prompttemplate.Resolve(prompttemplate.ResolveRequest{
		Template:          "让 {{@主体}} 穿着 {{$服装}}，再次参考 {{@主体}}；保留 \\{{@文本}}。",
		ReferenceAssetIDs: []string{"style-id", "subject-id", "unused-id"},
		ReferenceBindings: []prompttemplate.ReferenceBinding{{Name: "主体", AssetID: "subject-id"}},
		VariableBindings:  []prompttemplate.VariableBinding{{Name: "服装", Value: "蓝色风衣\n带银色纽扣"}},
		Assets: []prompttemplate.Asset{
			{ID: "style-id", Name: "风格"},
			{ID: "subject-id", Name: "主体"},
			{ID: "unused-id", Name: "构图"},
		},
		Limits: prompttemplate.DefaultLimits(),
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.Expanded != "让 图片2 穿着 蓝色风衣\n带银色纽扣，再次参考 图片2；保留 {{@文本}}。" {
		t.Fatalf("expanded = %q", result.Expanded)
	}
	if result.CanonicalTemplate != "让 {{@主体}} 穿着 {{$服装}}，再次参考 {{@主体}}；保留 \\{{@文本}}。" {
		t.Fatalf("canonical template = %q", result.CanonicalTemplate)
	}
	if len(result.Snapshot.References) != 1 || result.Snapshot.References[0].Index != 2 || result.Snapshot.References[0].AssetID != "subject-id" {
		t.Fatalf("reference snapshot = %#v", result.Snapshot.References)
	}
	if len(result.Snapshot.VariableNames) != 1 || result.Snapshot.VariableNames[0] != "服装" {
		t.Fatalf("variable snapshot = %#v", result.Snapshot.VariableNames)
	}
}

func TestResolveRejectsIncompleteExtraDuplicateAndStaleBindings(t *testing.T) {
	base := prompttemplate.ResolveRequest{
		Template:          "{{@主体}} {{$地点}}",
		ReferenceAssetIDs: []string{"asset-1"},
		ReferenceBindings: []prompttemplate.ReferenceBinding{{Name: "主体", AssetID: "asset-1"}},
		VariableBindings:  []prompttemplate.VariableBinding{{Name: "地点", Value: "上海"}},
		Assets:            []prompttemplate.Asset{{ID: "asset-1", Name: "主体"}},
		Limits:            prompttemplate.DefaultLimits(),
	}
	tests := []struct {
		name   string
		mutate func(*prompttemplate.ResolveRequest)
		code   string
		rule   string
	}{
		{name: "missing reference", mutate: func(req *prompttemplate.ResolveRequest) { req.ReferenceBindings = nil }, code: prompttemplate.CodeInvalid, rule: "reference_missing"},
		{name: "extra reference", mutate: func(req *prompttemplate.ResolveRequest) {
			req.ReferenceBindings = append(req.ReferenceBindings, prompttemplate.ReferenceBinding{Name: "风格", AssetID: "asset-1"})
		}, code: prompttemplate.CodeInvalid, rule: "reference_extra"},
		{name: "duplicate reference", mutate: func(req *prompttemplate.ResolveRequest) {
			req.ReferenceBindings = append(req.ReferenceBindings, req.ReferenceBindings[0])
		}, code: prompttemplate.CodeInvalid, rule: "reference_duplicate"},
		{name: "missing variable", mutate: func(req *prompttemplate.ResolveRequest) { req.VariableBindings = nil }, code: prompttemplate.CodeInvalid, rule: "variable_missing"},
		{name: "blank variable", mutate: func(req *prompttemplate.ResolveRequest) { req.VariableBindings[0].Value = "  \n " }, code: prompttemplate.CodeInvalid, rule: "variable_blank"},
		{name: "extra variable", mutate: func(req *prompttemplate.ResolveRequest) {
			req.VariableBindings = append(req.VariableBindings, prompttemplate.VariableBinding{Name: "天气", Value: "晴"})
		}, code: prompttemplate.CodeInvalid, rule: "variable_extra"},
		{name: "stale name", mutate: func(req *prompttemplate.ResolveRequest) { req.Assets[0].Name = "已改名" }, code: prompttemplate.CodeStale, rule: "reference_name_changed"},
		{name: "asset not selected", mutate: func(req *prompttemplate.ResolveRequest) { req.ReferenceBindings[0].AssetID = "asset-2" }, code: prompttemplate.CodeInvalid, rule: "reference_not_selected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := base
			req.ReferenceAssetIDs = append([]string(nil), base.ReferenceAssetIDs...)
			req.ReferenceBindings = append([]prompttemplate.ReferenceBinding(nil), base.ReferenceBindings...)
			req.VariableBindings = append([]prompttemplate.VariableBinding(nil), base.VariableBindings...)
			req.Assets = append([]prompttemplate.Asset(nil), base.Assets...)
			test.mutate(&req)
			_, err := prompttemplate.Resolve(req)
			resolveErr, ok := err.(*prompttemplate.Error)
			if !ok || resolveErr.Code != test.code || resolveErr.Rule != test.rule {
				t.Fatalf("error = %#v, want code=%q rule=%q", err, test.code, test.rule)
			}
		})
	}
}

func TestResolveDoesNotRecursivelyExpandVariableValues(t *testing.T) {
	result, err := prompttemplate.Resolve(prompttemplate.ResolveRequest{
		Template:         "{{$内容}}",
		VariableBindings: []prompttemplate.VariableBinding{{Name: "内容", Value: "{{@不会解析}} {{$也不会解析}}"}},
		Limits:           prompttemplate.DefaultLimits(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Expanded != "{{@不会解析}} {{$也不会解析}}" {
		t.Fatalf("expanded = %q", result.Expanded)
	}
}

func TestResolveEnforcesExpandedPromptLimit(t *testing.T) {
	limits := prompttemplate.DefaultLimits()
	limits.MaxExpandedRunes = 8
	_, err := prompttemplate.Resolve(prompttemplate.ResolveRequest{
		Template:         "前缀{{$内容}}",
		VariableBindings: []prompttemplate.VariableBinding{{Name: "内容", Value: strings.Repeat("字", 7)}},
		Limits:           limits,
	})
	resolveErr, ok := err.(*prompttemplate.Error)
	if !ok || resolveErr.Rule != "expanded_length" {
		t.Fatalf("error = %#v, want expanded_length", err)
	}
}
