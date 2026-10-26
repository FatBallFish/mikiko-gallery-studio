package mediaasset

import "testing"

func TestUploadDisplayNameFinalExtension(t *testing.T) {
	for input, want := range map[string]string{"猫.photo.PNG": "猫.photo", "plain": "plain", "archive.tar.gz": "archive.tar", `C:\fakepath\猫.png`: "猫", ".hidden": ".hidden"} {
		if got := UploadDisplayName(input); got != want {
			t.Fatalf("%q = %q, want %q", input, got, want)
		}
	}
}
func TestDeliveryFilenameIndependentOfDisplayName(t *testing.T) {
	object := assetStorageObject{ObjectKey: "original/a.png", MIMEType: "image/png"}
	for input, want := range map[string]string{"猫": "猫.png", "old.png": "old.png", "renamed.jpg": "renamed.jpg.png"} {
		if got := deliveryFilename(input, object); got != want {
			t.Fatalf("%q = %q, want %q", input, got, want)
		}
	}
}
