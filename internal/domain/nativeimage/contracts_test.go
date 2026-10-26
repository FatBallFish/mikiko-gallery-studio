package nativeimage

import "testing"

func TestModelProfilesAndSizes(t *testing.T) {
	for _, model := range []string{"qwen-image-3.0", "qwen-image-3.0-pro", "qwen-image-2.1-pro", "qwen-image-2.1-turbo", "doubao-seedream-5-0-pro-260628", "doubao-seedream-5-0-flash-260915"} {
		p, ok := ProfileForModel(model)
		if !ok {
			t.Fatal(model)
		}
		size := "2048x512"
		if p.Adapter == AdapterSeedream {
			size = "3072x384"
		}
		if err := ValidateSize(p, size, OperationGenerate); err != nil {
			t.Fatalf("wide native ratio: %s %v", model, err)
		}
		for _, bad := range []string{"1x1", "99999999x1", "1024x1024trailing", "dimensions", "-1x2048"} {
			if ValidateSize(p, bad, OperationGenerate) == nil {
				t.Errorf("accepted %s %s", model, bad)
			}
		}
	}
	p, _ := ProfileForModel("qwen-image-3.0")
	if p.MaxReferenceImages != 3 || p.MaxOutputImages != 6 || !p.NegativePrompt {
		t.Fatal(p)
	}
	p, _ = ProfileForModel("qwen-image-2.1-pro")
	if p.MaxReferenceImages != 10 || p.NegativePrompt {
		t.Fatal(p)
	}
	if _, ok := ProfileForModel("ep-unknown"); ok {
		t.Fatal("invented profile")
	}
}
func TestCoordinates(t *testing.T) {
	a := Annotation{ReferenceIndex: 1, Point: &[2]int{520, 460}}
	if a.PromptTag() != "<point>520 460</point>" {
		t.Fatal(a.PromptTag())
	}
	a = Annotation{ReferenceIndex: 2, BBox: &[4]int{120, 180, 640, 760}}
	if a.PromptTag() != "<bbox>120 180 640 760</bbox>" {
		t.Fatal(a.PromptTag())
	}
	p, _ := ProfileForModel("doubao-seedream-5-0-pro-260628")
	r := ValidationRequest{Operation: OperationEdit, Prompt: "x", ReferenceCount: 2, Options: &Options{Annotations: []Annotation{a}}}
	if err := ValidateRequest(p, r); err != nil {
		t.Fatal(err)
	}
	r.ReferenceCount = 1
	if ValidateRequest(p, r) == nil {
		t.Fatal("invalid reference index")
	}
	point, err := NormalizePoint(512, 1024, 1024, 1024)
	if err != nil || point != [2]int{500, 999} {
		t.Fatal(point, err)
	}
	if _, err = NormalizePoint(-1, 0, 1024, 1024); err == nil {
		t.Fatal("negative point")
	}
	for _, bad := range []Annotation{{}, {Point: &[2]int{1000, 0}}, {BBox: &[4]int{1, 2, 0, 3}}} {
		if bad.Validate() == nil {
			t.Fatal("invalid annotation")
		}
	}
}
func TestOperationsAndCounts(t *testing.T) {
	p, _ := ProfileForModel("doubao-seedream-5-0-pro-260628")
	for _, r := range []ValidationRequest{{Operation: OperationEdit, Prompt: "x"}, {Operation: OperationGenerate, Prompt: "x", ReferenceCount: 1}, {Operation: OperationLayerDecomposition, ReferenceCount: 2}, {Operation: OperationGenerate, Prompt: "x", Count: 2}, {Operation: OperationGenerate, Prompt: "x", Count: -1}, {Operation: "unknown", Prompt: "x"}, {Operation: OperationLayerDecomposition, ReferenceCount: 1, Size: "1024x1024"}} {
		if ValidateRequest(p, r) == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
	if err := ValidateRequest(p, ValidationRequest{Operation: OperationLayerDecomposition, ReferenceCount: 1, Size: "auto"}); err != nil {
		t.Fatal(err)
	}
}
func TestCNYRatesAndUnknownLayerPricing(t *testing.T) {
	for _, tc := range []struct {
		model                   string
		inputs, outputs, pixels int
		want                    float64
	}{{"qwen-image-3.0-pro", 2, 2, 1048576, .54}, {"qwen-image-3.0-pro", 1, 1, 4194304, .52}, {"qwen-image-3.0", 1, 1, 1048576, .20}, {"qwen-image-2.1-pro", 3, 1, 1048576, .25}, {"qwen-image-2.1-turbo", 2, 1, 1048576, .10}, {"doubao-seedream-5-0-pro-260628", 3, 1, 1048576, .34}, {"doubao-seedream-5-0-pro-260628", 1, 1, 4194304, .60}, {"doubao-seedream-5-0-flash-260915", 10, 1, 4194304, .12}} {
		p, ok := PricingForModel(tc.model)
		if !ok {
			t.Fatal(tc.model)
		}
		got, err := p.EstimateCNY(OperationEdit, tc.inputs, tc.outputs, tc.pixels)
		if err != nil || got-tc.want > 1e-9 || tc.want-got > 1e-9 {
			t.Fatalf("%s %v %v", tc.model, got, err)
		}
		if _, err = p.EstimateCNY(OperationLayerDecomposition, 1, 17, tc.pixels); err == nil {
			t.Fatalf("invented layer price for %s", tc.model)
		}
	}
}
