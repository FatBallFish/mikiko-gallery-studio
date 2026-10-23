package videomodeltest

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	videoprovider "github.com/fatballfish/pic-gallery/internal/provider/video"
)

type fakeLoader struct {
	account domainmodeladmin.ModelAccount
	model   domainmodeladmin.ModelAccountModel
}

func (f fakeLoader) GetModelAccount(context.Context, int64) (domainmodeladmin.ModelAccount, error) {
	return f.account, nil
}

func (f fakeLoader) GetModelAccountModel(context.Context, int64) (domainmodeladmin.ModelAccountModel, error) {
	return f.model, nil
}

type fakeProvider struct {
	status videoprovider.Status
}

func (f *fakeProvider) Submit(context.Context, videoprovider.Request) (videoprovider.Job, error) {
	return videoprovider.Job{ID: "job-1", State: videoprovider.StateQueued}, nil
}

func (f *fakeProvider) Get(context.Context, videoprovider.JobRef) (videoprovider.Status, error) {
	return f.status, nil
}

func (f *fakeProvider) Cancel(context.Context, videoprovider.JobRef) (videoprovider.CancelResult, error) {
	return videoprovider.CancelResult{}, errors.New("not implemented")
}

func (f *fakeProvider) VerifyCallback(context.Context, http.Header, []byte) (videoprovider.CallbackEvent, error) {
	return videoprovider.CallbackEvent{}, errors.New("not implemented")
}

func (f *fakeProvider) NormalizeUsage(videoprovider.Status) (videoprovider.Usage, error) {
	return videoprovider.Usage{}, nil
}

func TestSubmitPollsProviderUntilArtifactIsReady(t *testing.T) {
	loader := fakeLoader{
		account: domainmodeladmin.ModelAccount{
			ID: 1, AdapterType: "bailian", BaseURL: "https://example.test", TimeoutMS: 1000,
			CredentialsEncrypted: map[string]string{"api_key": "test-key"},
		},
		model: domainmodeladmin.ModelAccountModel{ID: 2, AccountID: 1, ModelCode: "wan-3.0", Enabled: true},
	}
	service := NewService(loader)
	service.pollInterval = 5 * time.Millisecond
	service.timeout = time.Second
	service.newProvider = func(domainmodeladmin.ModelAccount, domainmodeladmin.ModelAccountModel) (videoprovider.Provider, error) {
		return &fakeProvider{status: videoprovider.Status{
			JobID: "job-1", State: videoprovider.StateSucceeded,
			Artifacts: []videoprovider.Artifact{{URL: "https://cdn.example.test/result.mp4", MIMEType: "video/mp4", SizeBytes: 12}},
		}}, nil
	}

	test, err := service.Submit(context.Background(), Request{
		AccountID: 1, AccountModelID: 2, Prompt: "ocean", TaskType: "text_to_video",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		test, err = service.Get(test.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if test.Status == "succeeded" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if test.Status != "succeeded" {
		t.Fatalf("expected succeeded test, got %#v", test)
	}
	if len(test.Artifacts) != 1 || test.Artifacts[0].URL != "https://cdn.example.test/result.mp4" {
		t.Fatalf("expected artifact URL, got %#v", test.Artifacts)
	}
}

func TestSubmitRejectsModelFromAnotherAccount(t *testing.T) {
	service := NewService(fakeLoader{
		account: domainmodeladmin.ModelAccount{ID: 1, AdapterType: "bailian", BaseURL: "https://example.test", CredentialsEncrypted: map[string]string{"api_key": "test-key"}},
		model:   domainmodeladmin.ModelAccountModel{ID: 2, AccountID: 99, ModelCode: "wan-3.0", Enabled: true},
	})
	_, err := service.Submit(context.Background(), Request{AccountID: 1, AccountModelID: 2, Prompt: "ocean", TaskType: "text_to_video"})
	if err == nil {
		t.Fatal("expected model/account mismatch error")
	}
}
