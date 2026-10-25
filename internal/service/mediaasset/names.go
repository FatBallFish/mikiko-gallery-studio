package mediaasset

import (
	"mime"
	"path"
	"strings"
)

// UploadDisplayName strips only the final extension of new uploads.
func UploadDisplayName(filename string) string {
	name := path.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	stem := strings.TrimSuffix(name, path.Ext(name))
	if strings.TrimSpace(stem) == "" {
		return name
	}
	return stem
}

// deliveryFilename derives the extension from the delivered object, independently
// of the editable display title. Historical names already ending in it are kept.
func deliveryFilename(name string, object assetStorageObject) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "" || name == "." {
		name = "media"
	}
	ext := strings.ToLower(path.Ext(object.ObjectKey))
	if exts, _ := mime.ExtensionsByType(strings.Split(object.MIMEType, ";")[0]); len(exts) > 0 {
		matched := false
		for _, candidate := range exts {
			if strings.EqualFold(ext, candidate) {
				matched = true
			}
		}
		if !matched {
			ext = exts[0]
		}
	}
	if ext != "" && !strings.EqualFold(path.Ext(name), ext) {
		name += ext
	}
	return name
}
