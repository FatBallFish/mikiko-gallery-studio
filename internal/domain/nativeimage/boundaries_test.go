package nativeimage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
)

func encodedPNG(w, h int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h)))
	return b.Bytes()
}
func TestReferenceBoundaries(t *testing.T) {
	data := encodedPNG(512, 512)
	for _, tc := range []struct {
		adapter string
		op      Operation
		mime    string
		data    []byte
		valid   bool
	}{{AdapterQwen, OperationEdit, "image/png", data, true}, {AdapterSeedream, OperationLayerDecomposition, "image/png", data, true}, {AdapterQwen, OperationEdit, "image/jpeg", data, false}, {AdapterQwen, OperationEdit, "image/png", []byte("bad"), false}, {AdapterQwen, OperationEdit, "image/png", make([]byte, (10<<20)+1), false}, {AdapterSeedream, OperationEdit, "image/png", make([]byte, (30<<20)+1), false}, {AdapterQwen, OperationEdit, "image/png", encodedPNG(8001, 1), false}, {AdapterSeedream, OperationEdit, "image/png", encodedPNG(14, 512), false}, {AdapterSeedream, OperationLayerDecomposition, "image/png", encodedPNG(128, 128), false}, {AdapterQwen, OperationEdit, "image/png", data[:len(data)/2], false}} {
		err := ValidateReference(context.Background(), tc.adapter, tc.op, tc.mime, tc.data)
		if (err == nil) != tc.valid {
			t.Fatalf("%s/%s valid=%v err=%v", tc.adapter, tc.mime, tc.valid, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateReference(ctx, AdapterQwen, OperationEdit, "image/png", data); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestResponseOverflowAndCancellation(t *testing.T) {
	for _, n := range []int{7, 8, 9} {
		_, err := ReadResponse(context.Background(), strings.NewReader(strings.Repeat(" ", n)), 8)
		if (err != nil) != (n > 8) {
			t.Fatalf("n=%d err=%v", n, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadResponse(ctx, strings.NewReader("{}"), 8); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestStrictBBoxJSON(t *testing.T) {
	for _, s := range []string{`{"absolute":[0,0,1],"normalized":[0,0,1,1]}`, `{"absolute":[0,0,1,1,2],"normalized":[0,0,1,1]}`, `{"absolute":[0,null,1,1],"normalized":[0,0,1,1]}`, `{"absolute":[0,0,1,1]}`, `{"absolute":[0,0,1.5,1],"normalized":[0,0,1,1]}`} {
		var b BoundingBox
		if json.Unmarshal([]byte(s), &b) == nil {
			t.Fatal("accepted malformed bbox", s)
		}
	}
}
func TestProfileCannotExpand(t *testing.T) {
	p, _ := ProfileForModel("qwen-image-3.0")
	if err := ValidateProfile(p); err != nil {
		t.Fatal(err)
	}
	p.MaxReferenceImages = 10
	if ValidateProfile(p) == nil {
		t.Fatal("expanded profile")
	}
	p, _ = ProfileForModel("qwen-image-3.0")
	p.Model = "unknown"
	if ValidateProfile(p) == nil {
		t.Fatal("unknown profile")
	}
}
func TestUsagePrivacy(t *testing.T) {
	got := MeteringUsage(json.RawMessage(`{"generated_images":2,"url":"https://secret","output_tokens":"secret","input_images":-1}`))
	if string(got) != `{"generated_images":2}` {
		t.Fatal(string(got))
	}
}
