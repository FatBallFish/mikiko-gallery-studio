package imagetask

import (
	"context"
	"encoding/json"
	"fmt"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"strings"
)

func nativeSubmissionUncertain(err error) bool {
	if err == nil {
		return true
	}
	upstream, ok := provider.AsUpstreamError(err)
	// Without a concrete upstream rejection, submission may already be paid.
	return !ok || upstream.HTTPStatus == 0 || upstream.Code == "invalid_response"
}

func cloneNativeOptions(options *nativeimage.Options) *nativeimage.Options {
	if options == nil {
		return nil
	}
	raw, _ := json.Marshal(options)
	var clone nativeimage.Options
	_ = json.Unmarshal(raw, &clone)
	return &clone
}

func (s *Service) rejectUncertainSubmission(ctx context.Context, task image.Task, owner string) (image.ExecuteResult, error) {
	task.Status = image.StatusRejected
	task.ErrorCode = "IMAGE_UPSTREAM_SUBMISSION_UNCERTAIN"
	task.ErrorMessage = "上游提交结果不确定，需要人工对账；禁止自动重试"
	task.BillingDiagnostic = "manual_reconciliation_required: upstream_started_without_durable_response; reservation retained"
	setTaskProgress(&task, image.ProgressStageFailed, task.ErrorMessage)
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	if err := s.saveOwnedTask(ctx, task, owner); err != nil {
		return image.ExecuteResult{}, fmt.Errorf("persist uncertain submission: %w", err)
	}
	return image.ExecuteResult{Task: task}, errs.New(409, errs.CodeConflict, task.ErrorMessage)
}

func (s *Service) holdPaidSettlement(ctx context.Context, task image.Task, owner string, cause error) (image.ExecuteResult, error) {
	task.Status = image.StatusRunning
	task.ActualPoints = task.EstimatedPoints
	task.ProviderCost = ""
	task.GrossMargin = ""
	task.BillingDiagnostic = fmt.Sprintf("manual_reconciliation_required: settlement invalid: %v; authorized reservation charged, cost unknown", cause)
	task.ErrorCode = "IMAGE_SETTLEMENT_RECONCILIATION_REQUIRED"
	task.ErrorMessage = "生成结果已保存，按授权预留结算，费用需要人工核对"
	task.ArtifactRecovery = completedArtifactRecovery(task.ArtifactRecovery)
	setTaskProgress(&task, image.ProgressStageSettling, task.ErrorMessage)
	if err := s.saveOwnedTask(ctx, task, owner); err != nil {
		return image.ExecuteResult{}, fmt.Errorf("persist bounded settlement: %w", err)
	}
	if err := s.settleTaskBilling(ctx, task, "bounded settlement fallback; manual reconciliation required"); err != nil {
		return image.ExecuteResult{Task: task}, err
	}
	task.Status = image.StatusPartialFailed
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	setCompletedTaskProgress(&task)
	if err := s.saveOwnedTask(ctx, task, owner); err != nil {
		return image.ExecuteResult{}, err
	}
	return image.ExecuteResult{Task: task}, nil
}

func normalizeNativeCreate(req image.CreateRequest) (image.CreateRequest, error) {
	req.NativeOptions = cloneNativeOptions(req.NativeOptions)
	if req.Operation == "" {
		if req.TaskType == "image_edit" {
			req.Operation = nativeimage.OperationEdit
		} else {
			req.Operation = nativeimage.OperationGenerate
		}
	}
	switch req.Operation {
	case nativeimage.OperationGenerate:
		if req.TaskType != "text_to_image" {
			return req, errs.BadRequest("generate requires text_to_image task_type")
		}
	case nativeimage.OperationEdit, nativeimage.OperationLayerDecomposition:
		if req.TaskType != "image_edit" {
			return req, errs.BadRequest("native edit operation requires image_edit task_type")
		}
	default:
		return req, errs.BadRequest("unsupported image operation")
	}
	if req.Operation == nativeimage.OperationLayerDecomposition && req.OutputImageCount > 1 {
		return req, errs.BadRequest("layer decomposition is one operation; output_image_count must be 1")
	}
	if req.Seed != nil || strings.TrimSpace(req.NegativePrompt) != "" {
		if req.NativeOptions == nil {
			req.NativeOptions = &nativeimage.Options{}
		}
		if req.Seed != nil {
			if req.NativeOptions.Seed != nil && *req.NativeOptions.Seed != *req.Seed {
				return req, errs.BadRequest("conflicting native seed")
			}
			req.NativeOptions.Seed = req.Seed
		}
		if req.NegativePrompt != "" {
			if req.NativeOptions.NegativePrompt != "" && req.NativeOptions.NegativePrompt != req.NegativePrompt {
				return req, errs.BadRequest("conflicting negative_prompt")
			}
			req.NativeOptions.NegativePrompt = req.NegativePrompt
		}
	}
	return req, nil
}
