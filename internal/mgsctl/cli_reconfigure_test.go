package mgsctl

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunReconfigureResolvesRuntimeAndReportsResult(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			called := false
			code := Run(context.Background(), []string{"reconfigure", "--public-api-url", "https://mikiko.studio"}, CLIDependencies{
				Stdout: stdout, Stderr: stderr,
				ResolveRuntime: func(options RuntimeResolutionOptions) (string, error) {
					if options.Explicit {
						t.Fatal("runtime unexpectedly explicit")
					}
					return "/managed/studio", nil
				},
				ExecuteReconfigure: func(_ context.Context, options ReconfigureOptions, _ ReconfigureDependencies) (ReconfigureResult, error) {
					called = true
					if options.RuntimeDir != "/managed/studio" || options.PublicAPIURL != "https://mikiko.studio" {
						t.Fatalf("unexpected options: %#v", options)
					}
					if failed {
						return ReconfigureResult{}, errors.New("binding conflict")
					}
					return ReconfigureResult{ConfigRevision: 2, PublicAPIURL: options.PublicAPIURL}, nil
				},
			})
			if !called {
				t.Fatal("reconfigure was not executed")
			}
			if failed {
				if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "binding conflict") {
					t.Fatalf("unexpected failure: code=%d stdout=%q stderr=%q", code, stdout, stderr)
				}
			} else if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "installation binding are consistent") {
				t.Fatalf("unexpected success: code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}
