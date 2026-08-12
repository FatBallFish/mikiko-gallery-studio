package provider

import "testing"

func TestIsSupportedTaskTypeRejectsRemovedReferenceGeneration(t *testing.T) {
	for _, taskType := range []string{"reference_generate", "reference_to_image", "image_to_image", ""} {
		if IsSupportedTaskType(taskType) {
			t.Fatalf("removed task type %q must be rejected", taskType)
		}
	}
}

func TestIsSupportedTaskTypeAcceptsCurrentImageTasks(t *testing.T) {
	for _, taskType := range []string{"text_to_image", "image_edit", "text_to_video", "image_to_video", "first_last_frame_to_video"} {
		if !IsSupportedTaskType(taskType) {
			t.Fatalf("current task type %q must be accepted", taskType)
		}
	}
}
