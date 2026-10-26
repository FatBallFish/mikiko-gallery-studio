package mediaasset

import "testing"

func TestNormalizeVisibilityStatus(t *testing.T) {
	for input, want := range map[string]string{"": "", "all": "", "private": "private", " pending_review ": "pending", "reviewing": "pending", "pending": "pending", "approved": "public", "PUBLIC": "public", "unpublished": "offline", "offline": "offline", "rejected": "rejected"} {
		got, err := NormalizeVisibilityStatus(input)
		if err != nil || got != want {
			t.Errorf("normalize %q = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := NormalizeVisibilityStatus("invalid"); err == nil {
		t.Fatal("invalid status accepted")
	}
}
