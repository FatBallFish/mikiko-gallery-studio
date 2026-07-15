package imagetask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

const (
	artifactRecoveryPending    = "pending"
	artifactRecoveryPersisting = "persisting"
)

func (s *Service) checkpointProviderSuccess(
	ctx context.Context,
	task *domainimagetask.Task,
	owner string,
	candidate modelhub.ProviderCandidate,
	response provider.ImageResponse,
	startedAt time.Time,
	finishedAt time.Time,
) error {
	if task == nil {
		return fmt.Errorf("checkpoint provider success: task is nil")
	}
	payload, err := s.encryptArtifactResults(response.Data)
	if err != nil {
		return fmt.Errorf("encrypt artifact recovery payload: %w", err)
	}
	writer, err := s.router.DefaultWriter(ctx)
	if err != nil {
		return fmt.Errorf("resolve artifact storage writer: %w", err)
	}
	decorated := s.decorateTaskProvider(*task, candidate)
	decorated.Status = domainimagetask.StatusRunning
	decorated.ProviderRequestID = strings.TrimSpace(response.ProviderRequestID)
	completedAt := finishedAt.UTC()
	decorated.UpstreamSucceededAt = &completedAt
	decorated.Attempts = append(decorated.Attempts, buildProviderAttempt(candidate, domainimagetask.StatusSucceeded, nil, startedAt, finishedAt))
	decorated.ArtifactRecovery = domainimagetask.ArtifactRecovery{
		Status:           artifactRecoveryPersisting,
		EncryptedPayload: payload,
		StorageConfigID:  writer.ConfigID,
		StorageVersion:   writer.Version,
	}
	if err := s.saveOwnedTask(ctx, decorated, owner); err != nil {
		// A paid upstream result must survive an expired lease. SaveTerminalState
		// preserves the current owner's lease columns while durably recording the
		// recovery envelope, allowing the current or reclaiming worker to resume it.
		if snapshotErr := s.saveTerminalState(ctx, decorated, owner); snapshotErr != nil {
			return fmt.Errorf("persist provider success checkpoint: %w", err)
		}
	}
	*task = decorated
	return nil
}

func (s *Service) encryptArtifactResults(results []provider.ImageResult) (string, error) {
	if s == nil || s.recoveryCodec == nil {
		return "", fmt.Errorf("artifact recovery codec is unavailable")
	}
	envelope, err := s.recoveryCodec.EncryptJSON(map[string]any{"results": results})
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *Service) decryptArtifactResults(payload string) ([]provider.ImageResult, error) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, err
	}
	decoded, err := s.recoveryCodec.DecryptJSON(envelope)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(decoded["results"])
	if err != nil {
		return nil, err
	}
	var results []provider.ImageResult
	if err := json.Unmarshal(raw, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Service) artifactWriter(ctx context.Context, task domainimagetask.Task) (storage.BackendRef, error) {
	if strings.TrimSpace(task.ArtifactRecovery.StorageConfigID) != "" {
		return s.router.BackendFor(ctx, task.ArtifactRecovery.StorageConfigID, "")
	}
	return s.router.DefaultWriter(ctx)
}
