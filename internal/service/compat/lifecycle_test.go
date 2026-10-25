package compat

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/service/capabilities"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type operationFunc func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error)

func (f operationFunc) Execute(ctx context.Context, p openplatform.Principal, op string, in json.RawMessage) (any, error) {
	return f(ctx, p, op, in)
}

func TestLifecycleStartSurvivesCanceledTransport(t *testing.T) {
	s := NewService(config.Config{})
	s.SetOperations(operationFunc(func(ctx context.Context, p openplatform.Principal, op string, in json.RawMessage) (any, error) {
		if ctx.Err() != nil {
			t.Fatal("creation inherited disconnect")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("creation is unbounded")
		}
		if op != "create_image_task" {
			t.Fatal(op)
		}
		return domainimagetask.Task{ID: "durable", Status: domainimagetask.StatusQueued}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task, err := s.Start(ctx, openplatform.Principal{UserID: 7}, map[string]string{"task_id": "durable"})
	if err != nil || task.ID != "durable" {
		t.Fatalf("%+v %v", task, err)
	}
}
func TestLifecycleWaitDisconnectNeverCreatesOrCancelsTask(t *testing.T) {
	s := NewService(config.Config{})
	calls := 0
	s.SetOperations(operationFunc(func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error) {
		calls++
		return nil, errors.New("unexpected mutation")
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task, err := s.Wait(ctx, openplatform.Principal{}, domainimagetask.Task{ID: "saved", Status: domainimagetask.StatusRunning}, nil)
	if !errors.Is(err, context.Canceled) || calls != 0 || task.ID != "saved" {
		t.Fatalf("task=%+v err=%v calls=%d", task, err, calls)
	}
}
func TestLifecycleCompletedReplayUsesSavedResultsOnly(t *testing.T) {
	s := NewService(config.Config{})
	s.SetOperations(operationFunc(func(context.Context, openplatform.Principal, string, json.RawMessage) (any, error) {
		t.Fatal("completed result reexecuted")
		return nil, nil
	}))
	task := domainimagetask.Task{ID: "saved", Status: domainimagetask.StatusSucceeded, CreatedAt: time.Unix(12, 0), Results: []provider.ImageResult{{B64JSON: "saved-bytes", URL: "saved-url", RevisedPrompt: "saved-prompt"}}}
	task, err := s.Wait(context.Background(), openplatform.Principal{}, task, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Result(context.Background(), openplatform.Principal{}, task, "b64_json")
	if err != nil || result.Data[0].B64JSON != "saved-bytes" {
		t.Fatalf("%+v %v", result, err)
	}
	raw, _ := json.Marshal(result)
	var shape map[string]any
	_ = json.Unmarshal(raw, &shape)
	if len(shape) != 2 || shape["created"] != float64(12) {
		t.Fatal(string(raw))
	}
}
func TestLifecycleModelsUseVisibleImageCapabilities(t *testing.T) {
	s := NewService(config.Config{})
	s.SetOperations(operationFunc(func(_ context.Context, p openplatform.Principal, op string, _ json.RawMessage) (any, error) {
		if op != "list_image_models" || p.GroupCode != "restricted" {
			t.Fatalf("%s %+v", op, p)
		}
		return capabilities.Response{ModelGroups: []modelhub.VisibleRouteModel{{Code: "image-live", TaskTypes: []string{string(provider.TaskTypeTextToImage)}}, {Code: "video-only", TaskTypes: []string{"text_to_video"}}}}, nil
	}))
	result, err := s.ModelsForPrincipal(context.Background(), openplatform.Principal{GroupCode: "restricted"})
	if err != nil || len(result.Data) != 1 || result.Data[0].ID != "image-live" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestLifecycleStartNegotiatesQuoteOnlyForNewTask(t *testing.T) {
	s := NewService(config.Config{})
	ops := []string{}
	s.SetOperations(operationFunc(func(ctx context.Context, p openplatform.Principal, op string, input json.RawMessage) (any, error) {
		ops = append(ops, op)
		if len(ops) == 1 {
			return nil, errs.New(409, "QUOTE_STALE", "quote required")
		}
		if op == "preflight_image_task" {
			return map[string]string{"quote_token": "bound"}, nil
		}
		var create map[string]any
		_ = json.Unmarshal(input, &create)
		if create["quote_token"] != "bound" {
			t.Fatal(create)
		}
		return domainimagetask.Task{ID: "durable"}, nil
	}))
	task, err := s.Start(context.Background(), openplatform.Principal{}, map[string]string{"idempotency_key": "stable"})
	if err != nil || task.ID != "durable" || len(ops) != 3 || ops[1] != "preflight_image_task" {
		t.Fatalf("%+v %v %v", task, err, ops)
	}
}
