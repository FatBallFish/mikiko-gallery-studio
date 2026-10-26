package compat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

// Operations is the same authenticated application boundary used by REST and MCP.
type Operations interface {
	Execute(context.Context, openplatform.Principal, string, json.RawMessage) (any, error)
}

func (s *Service) SetOperations(operations Operations) { s.operations = operations }

func (s *Service) operation(ctx context.Context, p openplatform.Principal, op string, input any, output any) error {
	if s.operations == nil {
		return errs.New(503, "COMPAT_UNAVAILABLE", "shared developer operations are unavailable")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	value, err := s.operations.Execute(ctx, p, op, raw)
	if err != nil {
		return err
	}
	raw, err = json.Marshal(value)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("decode %s result: %w", op, err)
	}
	return nil
}

// Start creates a durable native task. Only the bounded creation transaction is
// detached from the transport; waiting never owns the worker's lifetime.
func (s *Service) Start(ctx context.Context, p openplatform.Principal, input any) (domainimagetask.Task, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	var task domainimagetask.Task
	err := s.operation(ctx, p, "create_image_task", input, &task)
	if err != nil && MapError(err).Code == "QUOTE_STALE" {
		var quote struct {
			Token string `json:"quote_token"`
		}
		if err = s.operation(ctx, p, "preflight_image_task", input, &quote); err != nil {
			return task, err
		}
		raw, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			return task, marshalErr
		}
		var create map[string]any
		if err = json.Unmarshal(raw, &create); err != nil {
			return task, err
		}
		create["quote_token"] = quote.Token
		err = s.operation(ctx, p, "create_image_task", create, &task)
	}
	return task, err
}
func (s *Service) Task(ctx context.Context, p openplatform.Principal, id string) (domainimagetask.Task, error) {
	var task domainimagetask.Task
	err := s.operation(ctx, p, "get_image_task", map[string]string{"id": id}, &task)
	return task, err
}

func Terminal(task domainimagetask.Task) bool {
	switch task.Status {
	case domainimagetask.StatusSucceeded, domainimagetask.StatusPartialFailed, domainimagetask.StatusFailed, domainimagetask.StatusRejected, domainimagetask.StatusDeleted:
		return true
	}
	return false
}

// Wait observes existing state only; it never retries generation or cancels tasks.
func (s *Service) Wait(ctx context.Context, p openplatform.Principal, task domainimagetask.Task, observe func(domainimagetask.Task) error) (domainimagetask.Task, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if observe != nil {
			if err := observe(task); err != nil {
				return task, err
			}
		}
		if Terminal(task) {
			return task, nil
		}
		select {
		case <-ctx.Done():
			return task, ctx.Err()
		case <-ticker.C:
		}
		next, err := s.Task(ctx, p, task.ID)
		if err != nil {
			return task, err
		}
		task = next
	}
}

type Image struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}
type Response struct {
	Created int64   `json:"created"`
	Data    []Image `json:"data"`
}

// Result reads saved artifacts using the existing storage delivery path. A retry
// can renew URLs or encode stored bytes but can never call a provider again.
func (s *Service) Result(ctx context.Context, p openplatform.Principal, task domainimagetask.Task, format string) (Response, error) {
	response := Response{Created: task.CreatedAt.Unix(), Data: make([]Image, 0, len(task.Results))}
	if len(task.Results) == 0 {
		return response, errs.New(503, firstNonEmpty(task.ErrorCode, errs.CodeUpstreamUnavailable), firstNonEmpty(task.ErrorMessage, "image task completed without images"))
	}
	for _, result := range task.Results {
		item := Image{RevisedPrompt: result.RevisedPrompt}
		if format == "b64_json" {
			item.B64JSON = result.B64JSON
			if item.B64JSON == "" {
				_, content, err := s.taskSvc.DownloadImageResult(ctx, p.UserID, result.ID)
				if err != nil {
					return response, err
				}
				item.B64JSON = base64.StdEncoding.EncodeToString(content)
			}
		} else {
			assetID := firstNonEmpty(result.MediaAssetID, result.ID)
			if assetID != "" {
				var access struct {
					URL string `json:"url"`
				}
				if err := s.operation(ctx, p, "get_image_result_access", map[string]string{"task_id": task.ID, "image_id": result.ID, "purpose": "download"}, &access); err != nil {
					return response, err
				}
				item.URL = access.URL
			} else {
				item.URL = result.URL
			}
		}
		response.Data = append(response.Data, item)
	}
	return response, nil
}

func (s *Service) ModelsForPrincipal(ctx context.Context, p openplatform.Principal) (ModelsResponse, error) {
	var raw json.RawMessage
	if err := s.operation(ctx, p, "list_image_models", map[string]any{}, &raw); err != nil {
		return ModelsResponse{}, err
	}
	var envelope struct {
		ModelGroups []modelhub.VisibleRouteModel `json:"model_groups"`
		Items       []modelhub.VisibleRouteModel `json:"items"`
	}
	var models []modelhub.VisibleRouteModel
	if err := json.Unmarshal(raw, &models); err != nil {
		if err = json.Unmarshal(raw, &envelope); err != nil {
			return ModelsResponse{}, err
		}
		models = append(envelope.ModelGroups, envelope.Items...)
	}
	response := ModelsResponse{Object: "list", Data: []ModelItem{}}
	seen := map[string]bool{}
	for _, model := range models {
		image := false
		for _, kind := range model.TaskTypes {
			if kind == "text_to_image" || kind == "image_edit" || kind == "image_to_image" {
				image = true
			}
		}
		if image && model.Code != "" && !seen[model.Code] {
			seen[model.Code] = true
			response.Data = append(response.Data, ModelItem{ID: model.Code, Object: "model", OwnedBy: defaultString(s.cfg.App.Name, "pic-gallery")})
		}
	}
	sort.Slice(response.Data, func(i, j int) bool { return response.Data[i].ID < response.Data[j].ID })
	return response, nil
}
