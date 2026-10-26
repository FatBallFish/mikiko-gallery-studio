package imagetitle

import (
	"context"
	"errors"
	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
	"regexp"
	"strings"
	"testing"
	"time"
)

type resolverStub struct{ err error }

func (s resolverStub) ResolveDefaultModel(context.Context) (domaintextmodel.AccountRecord, domaintextmodel.Model, string, error) {
	return domaintextmodel.AccountRecord{}, domaintextmodel.Model{ModelCode: "default-model"}, "key", s.err
}

type optimizerFunc func(context.Context, textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error)

func (f optimizerFunc) Optimize(ctx context.Context, req textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error) {
	return f(ctx, req)
}
func TestGenerateDefaultModelBoundedUnicodeTitle(t *testing.T) {
	svc := NewService(resolverStub{})
	svc.factory = func(_ domaintextmodel.AccountRecord, key string) (textprovider.Optimizer, error) {
		if key != "key" {
			t.Fatal(key)
		}
		return optimizerFunc(func(ctx context.Context, req textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > Timeout {
				t.Fatal("missing bounded context")
			}
			if req.Model != "default-model" || len([]rune(req.Prompt)) != 2000 || req.MaxOutputTokens != 512 {
				t.Fatalf("request: %+v", req)
			}
			return textprovider.OptimizeResponse{Text: "“一二三四五六七八九十一二三四五六七八九十”"}, nil
		}), nil
	}
	title := svc.Generate(t.Context(), strings.Repeat("画", 3000))
	if len([]rune(title)) != 15 || strings.Contains(title, "“") {
		t.Fatal(title)
	}
}
func TestGenerateFailureAndCancellationAreNonfatal(t *testing.T) {
	if got := NewService(resolverStub{err: errors.New("no default")}).Generate(t.Context(), "prompt"); got != "" {
		t.Fatal(got)
	}
	svc := NewService(resolverStub{})
	svc.factory = func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) {
		return optimizerFunc(func(ctx context.Context, _ textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error) {
			<-ctx.Done()
			return textprovider.OptimizeResponse{}, ctx.Err()
		}), nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got := svc.Generate(ctx, "prompt"); got != "" {
		t.Fatal(got)
	}
}
func TestSanitizeAndFallback(t *testing.T) {
	if got := Sanitize(" \"猫/\\\n\u200b咪\" "); got != "猫咪" {
		t.Fatalf("%q", got)
	}
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	seen := map[string]bool{}
	for range 100 {
		name := Fallback(now)
		if !regexp.MustCompile(`^20261024-[A-Za-z0-9]{5}$`).MatchString(name) || seen[name] {
			t.Fatal(name)
		}
		seen[name] = true
	}
}
