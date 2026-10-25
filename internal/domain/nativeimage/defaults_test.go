package nativeimage

import (
	"encoding/json"
	"testing"
)

func TestDefaultsEffectivePolicy(t *testing.T) {
	q, _ := ProfileForModel("qwen-image-3.0")
	o, err := EffectiveOptions(q, nil, nil)
	if err != nil || *o.Watermark || *o.Qwen.PromptExtend || *o.Qwen.EnableThinking || o.Seed != nil {
		t.Fatalf("unsafe defaults: %+v %v", o, err)
	}
	yes, no := true, false
	d := &Defaults{Qwen: &QwenOptions{PromptExtend: &yes}}
	o, err = EffectiveOptions(q, d, nil)
	if err != nil || o.Qwen.PromptExtendMode != "direct" {
		t.Fatalf("direct extension: %+v %v", o, err)
	}
	o, err = EffectiveOptions(q, d, &Options{Watermark: &yes, Qwen: &QwenOptions{PromptExtend: &no}})
	if err != nil || *o.Qwen.PromptExtend || !*o.Watermark {
		t.Fatalf("explicit override: %+v %v", o, err)
	}
	if _, err = EffectiveOptions(q, nil, &Options{Qwen: &QwenOptions{EnableThinking: &yes}}); err == nil {
		t.Fatal("thinking without extension accepted")
	}
	s, _ := ProfileForModel("doubao-seedream-5-0-flash-260915")
	o, err = EffectiveOptions(s, nil, nil)
	if err != nil || o.Seedream.OptimizePromptMode != "standard" || *o.Watermark {
		t.Fatalf("Seedream defaults: %+v %v", o, err)
	}
	for _, mode := range []string{"off", "fast"} {
		if err := (&Defaults{Seedream: &SeedreamOptions{OptimizePromptMode: mode}}).Validate(s); err == nil {
			t.Fatalf("unsupported %s accepted", mode)
		}
	}
	seed := int64(0)
	if err := (&Defaults{Seed: &seed}).Validate(s); err == nil {
		t.Fatal("Seedream seed accepted")
	}
}

func TestDefaultsStrictFieldsAndInheritance(t *testing.T) {
	var d Defaults
	if err := json.Unmarshal([]byte(`{"watermark":true,"qwen":{"prompt_extend":true,"enable_thinking":true}}`), &d); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"qwen":{"enable_thinking":false}}`), &d); err != nil {
		t.Fatal(err)
	}
	if !*d.Watermark || !*d.Qwen.PromptExtend || *d.Qwen.EnableThinking {
		t.Fatalf("lost inherited fields: %+v", d)
	}
	for _, raw := range []string{`{"negative_prompt":"x"}`, `{"qwen":{"invented":true}}`, `{"seed":1.5}`} {
		if err := json.Unmarshal([]byte(raw), &d); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
