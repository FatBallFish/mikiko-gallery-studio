package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

type Config struct {
	Address            string
	APIURL             *url.URL
	FrontendAPIBaseURL string
	DocsURL            string
	UserDir            string
	AdminDir           string
	DocsDir            string
}

type handler struct {
	proxy         *httputil.ReverseProxy
	runtimeConfig []byte
	userDir       string
	adminDir      string
	docsDir       string
}

type requestContextKey uint8

const noStoreContextKey requestContextKey = iota

var immutableAssetPattern = regexp.MustCompile(`-[A-Za-z0-9_]{8,}\.[A-Za-z0-9]+$`)

func NewHandler(config Config) (http.Handler, error) {
	if config.APIURL == nil || (config.APIURL.Scheme != "http" && config.APIURL.Scheme != "https") || config.APIURL.Host == "" || config.APIURL.Hostname() == "" || config.APIURL.User != nil || config.APIURL.RawQuery != "" || config.APIURL.ForceQuery || config.APIURL.Fragment != "" {
		return nil, fmt.Errorf("Gateway API URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	for name, directory := range map[string]string{"user": config.UserDir, "admin": config.AdminDir, "docs": config.DocsDir} {
		if strings.TrimSpace(directory) == "" {
			return nil, fmt.Errorf("Gateway %s asset directory is required", name)
		}
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("validate Gateway %s asset directory: %w", name, err)
		}
	}
	if config.DocsURL == "" {
		config.DocsURL = "/developer-docs/"
	}
	publicConfig, err := renderRuntimeConfig(config.FrontendAPIBaseURL, config.DocsURL)
	if err != nil {
		return nil, err
	}
	target := cloneURL(config.APIURL)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.SetXForwarded()
			request.Out.Host = target.Host
		},
		ErrorHandler: func(writer http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(writer, "upstream unavailable", http.StatusBadGateway)
		},
		ModifyResponse: func(response *http.Response) error {
			if noStore, _ := response.Request.Context().Value(noStoreContextKey).(bool); noStore {
				response.Header.Set("Cache-Control", "no-store")
			}
			return nil
		},
	}
	return &handler{
		proxy: proxy, runtimeConfig: publicConfig,
		userDir: config.UserDir, adminDir: config.AdminDir, docsDir: config.DocsDir,
	}, nil
}

func Serve(ctx context.Context, listener net.Listener, config Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if listener == nil {
		return fmt.Errorf("Gateway listener is required")
	}
	httpHandler, err := NewHandler(config)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           httpHandler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	group, groupContext := errgroup.WithContext(ctx)
	group.Go(func() error {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	group.Go(func() error {
		<-groupContext.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	})
	return group.Wait()
}

func ListenAndServe(ctx context.Context, config Config) error {
	address := strings.TrimSpace(config.Address)
	if address == "" {
		return fmt.Errorf("Gateway listen address is required")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen for Gateway traffic: %w", err)
	}
	defer listener.Close()
	return Serve(ctx, listener, config)
}

func (gateway *handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if unsafeRequestPath(request.URL) {
		http.Error(writer, "invalid path", http.StatusBadRequest)
		return
	}
	if isAPIPath(request.URL.Path) {
		if noStoreAPIPath(request.URL.Path) {
			request = request.WithContext(context.WithValue(request.Context(), noStoreContextKey, true))
		}
		gateway.proxy.ServeHTTP(writer, request)
		return
	}
	switch request.URL.Path {
	case "/env.js", "/admin/env.js":
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.WriteHeader(http.StatusOK)
		if request.Method != http.MethodHead {
			_, _ = writer.Write(gateway.runtimeConfig)
		}
		return
	case "/admin":
		http.Redirect(writer, request, redirectLocation("/admin/", request.URL.RawQuery), http.StatusPermanentRedirect)
		return
	case "/developer-docs":
		http.Redirect(writer, request, redirectLocation("/developer-docs/", request.URL.RawQuery), http.StatusPermanentRedirect)
		return
	}
	if strings.HasPrefix(request.URL.Path, "/admin/") {
		gateway.serveStatic(writer, request, gateway.adminDir, "/admin/")
		return
	}
	if strings.HasPrefix(request.URL.Path, "/developer-docs/") {
		gateway.serveStatic(writer, request, gateway.docsDir, "/developer-docs/")
		return
	}
	gateway.serveStatic(writer, request, gateway.userDir, "/")
}

func (gateway *handler) serveStatic(writer http.ResponseWriter, request *http.Request, directory, prefix string) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(request.URL.Path, prefix)
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "index.html"
	}
	if strings.HasSuffix(name, "/") {
		http.NotFound(writer, request)
		return
	}
	file, info, err := openStaticFile(directory, name)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			http.Error(writer, "invalid asset path", http.StatusBadRequest)
			return
		}
		if !spaFallbackAllowed(name) {
			http.NotFound(writer, request)
			return
		}
		name = "index.html"
		file, info, err = openStaticFile(directory, name)
	}
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	defer file.Close()
	if name == "index.html" {
		writer.Header().Set("Cache-Control", "no-store")
	} else if immutableAssetPattern.MatchString(path.Base(name)) {
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		writer.Header().Set("Cache-Control", "no-cache")
	}
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		writer.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(writer, request, name, info.ModTime(), file)
}

func openStaticFile(directory, name string) (*os.File, os.FileInfo, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, nil, fs.ErrInvalid
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fs.ErrNotExist
	}
	return file, info, nil
}

func renderRuntimeConfig(apiBaseURL, docsURL string) ([]byte, error) {
	apiConfig, err := json.Marshal(map[string]string{"apiBaseUrl": apiBaseURL})
	if err != nil {
		return nil, fmt.Errorf("render frontend API config: %w", err)
	}
	userConfig, err := json.Marshal(map[string]string{"VITE_DOCS_URL": docsURL})
	if err != nil {
		return nil, fmt.Errorf("render frontend environment: %w", err)
	}
	return []byte("window.__PIC_GALLERY_CONFIG__ = " + string(apiConfig) + ";\nwindow.__PIC_GALLERY_ENV__ = " + string(userConfig) + ";\n"), nil
}

func unsafeRequestPath(requestURL *url.URL) bool {
	if requestURL == nil {
		return true
	}
	escaped := strings.ToLower(requestURL.EscapedPath())
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") {
		return true
	}
	candidate := requestURL.Path
	for range 3 {
		if unsafeDecodedPath(candidate) {
			return true
		}
		decoded, err := url.PathUnescape(candidate)
		if err != nil || decoded == candidate {
			break
		}
		candidate = decoded
	}
	return false
}

func unsafeDecodedPath(value string) bool {
	if strings.ContainsAny(value, "\\\x00") {
		return true
	}
	segments := strings.Split(value, "/")
	for index, segment := range segments {
		if segment == "." || segment == ".." || strings.HasPrefix(segment, ".") {
			return true
		}
		if index == 1 && (segment == "config" || segment == "data" || segment == "logs") {
			return true
		}
	}
	return false
}

func isAPIPath(requestPath string) bool {
	switch requestPath {
	case "/healthz", "/readyz", "/setup", "/api", "/metrics", "/docs", "/v1", "/files":
		return true
	}
	for _, prefix := range []string{"/setup/", "/api/", "/metrics/", "/docs/", "/v1/", "/files/"} {
		if strings.HasPrefix(requestPath, prefix) {
			return true
		}
	}
	return false
}

func noStoreAPIPath(requestPath string) bool {
	return requestPath == "/setup" || strings.HasPrefix(requestPath, "/setup/") || requestPath == "/api/system/v1/bootstrap-status" || strings.HasPrefix(requestPath, "/api/setup/")
}

func spaFallbackAllowed(name string) bool {
	if name == "" || strings.HasSuffix(name, "/") || path.Ext(path.Base(name)) != "" {
		return false
	}
	first, _, _ := strings.Cut(name, "/")
	return first != "assets" && first != "openapi"
}

func redirectLocation(destination, rawQuery string) string {
	if rawQuery == "" {
		return destination
	}
	return destination + "?" + rawQuery
}

func cloneURL(value *url.URL) *url.URL {
	cloned := *value
	return &cloned
}
