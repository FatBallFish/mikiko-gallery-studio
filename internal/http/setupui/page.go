package setupui

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
)

type Page struct {
	document []byte
	csp      string
}

func NewPage(model Model) (*Page, error) {
	modelJSON := model.JSON()
	if len(modelJSON) == 0 || string(modelJSON) == "null" {
		return nil, fmt.Errorf("encode setup page model")
	}
	styles := adminDesignTokensCSS + "\n" + setupPageCSS
	document := []byte(setupDocumentStart + styles + setupDocumentBody + string(modelJSON) + setupDocumentScriptOpen + setupPageScript + setupDocumentEnd)
	return &Page{
		document: document,
		csp: "default-src 'none'; base-uri 'none'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; img-src 'none'; font-src 'none'; style-src '" +
			cspContentHash(styles) + "'; script-src '" + cspContentHash(setupPageScript) + "' '" + cspContentHash(string(modelJSON)) + "'",
	}, nil
}

func (page *Page) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", page.csp)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page.document)
}

func cspContentHash(content string) string {
	digest := sha256.Sum256([]byte(content))
	return "sha256-" + base64.StdEncoding.EncodeToString(digest[:])
}
