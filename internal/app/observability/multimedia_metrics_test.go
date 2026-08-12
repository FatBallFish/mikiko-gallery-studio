package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsExposeBoundedMultimediaOperations(t *testing.T) {
	metrics := NewMetrics()
	metrics.RecordVideoStage("provider_running", "success")
	metrics.RecordArtifactTransfer("video", "success", 4096)
	metrics.RecordSettlement("video", "success")
	metrics.RecordUpload("complete", "success", 8192)
	metrics.RecordDerivative("proxy", "success", 2048)
	metrics.RecordCanvasSave("success")
	metrics.RecordMediaAssetBackfill("processed")
	metrics.RecordMediaAssetBackfill("completed")
	metrics.SetTemporaryDisk(76, 10<<30)
	metrics.AddObjectBytes("written", 6144)

	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, expected := range []string{
		`pic_gallery_video_stage_total{stage="provider_running",result="success"} 1`,
		`pic_gallery_artifact_transfer_bytes_total{media_type="video",result="success"} 4096`,
		`pic_gallery_settlement_total{kind="video",result="success"} 1`,
		`pic_gallery_upload_bytes_total{stage="complete",result="success"} 8192`,
		`pic_gallery_media_derivative_bytes_total{kind="proxy",result="success"} 2048`,
		`pic_gallery_canvas_save_total{result="success"} 1`,
		`pic_gallery_media_asset_backfill_total{result="completed"} 1`,
		`pic_gallery_media_asset_backfill_total{result="processed"} 1`,
		`pic_gallery_worker_temporary_disk_used_percent 76`,
		`pic_gallery_worker_temporary_disk_free_bytes 10737418240`,
		`pic_gallery_object_bytes_total{operation="written"} 6144`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("metrics missing %q:\n%s", expected, body)
		}
	}
}

func TestMetricsNormalizeUnknownLabels(t *testing.T) {
	metrics := NewMetrics()
	metrics.RecordVideoStage("task-secret-123", "credential-secret")
	metrics.RecordMediaAssetBackfill("asset-secret-123")
	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	if strings.Contains(body, "secret") || !strings.Contains(body, `stage="unknown",result="unknown"`) || !strings.Contains(body, `pic_gallery_media_asset_backfill_total{result="unknown"} 1`) {
		t.Fatalf("unbounded label exposed: %s", body)
	}
}
