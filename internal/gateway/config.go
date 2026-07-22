package gateway

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func ConfigFromBootstrap(bootstrap config.BootstrapConfig, runtimeDirectory string) (Config, error) {
	if !slices.Contains(bootstrap.DeploymentModules, "gateway") {
		return Config{}, fmt.Errorf("runtime deployment does not include the Gateway module")
	}
	listenPort, err := gatewayPort(bootstrap.Values["GATEWAY_PORT"])
	if err != nil {
		return Config{}, err
	}
	var apiURL *url.URL
	if bootstrap.Deployment.Role == config.DeploymentRoleWeb {
		apiURL, err = parseAPIURL(bootstrap.Values["PUBLIC_API_URL"])
		if err != nil {
			return Config{}, err
		}
	} else {
		apiPort, portErr := gatewayPort(bootstrap.Values["API_PORT"])
		if portErr != nil {
			return Config{}, fmt.Errorf("API port: %w", portErr)
		}
		apiURL, _ = url.Parse("http://127.0.0.1:" + apiPort)
	}
	root := filepath.Clean(runtimeDirectory)
	if strings.TrimSpace(runtimeDirectory) == "" {
		root = "."
	}
	return Config{
		Address: ":" + listenPort, APIURL: apiURL, FrontendAPIBaseURL: "", DocsURL: "/developer-docs/",
		UserDir: frontendDirectory(root, "user"), AdminDir: frontendDirectory(root, "admin"), DocsDir: frontendDirectory(root, "docs"),
	}, nil
}

func frontendDirectory(root, name string) string {
	directory := filepath.Join(root, "web", name)
	if info, err := os.Stat(filepath.Join(directory, "index.html")); err == nil && info.Mode().IsRegular() {
		return directory
	}
	distDirectory := filepath.Join(directory, "dist")
	if info, err := os.Stat(filepath.Join(distDirectory, "index.html")); err == nil && info.Mode().IsRegular() {
		return distDirectory
	}
	return directory
}

func gatewayPort(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	port, err := strconv.Atoi(trimmed)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535")
	}
	return strconv.Itoa(port), nil
}

func parseAPIURL(value string) (*url.URL, error) {
	raw := strings.TrimSpace(value)
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.ContainsAny(raw, "\\?#") {
		return nil, fmt.Errorf("PUBLIC_API_URL must be an absolute HTTP(S) URL without credentials, query, fragment, or backslashes")
	}
	if _, err := gatewayPort(defaultPort(parsed)); err != nil {
		return nil, fmt.Errorf("PUBLIC_API_URL has an invalid port")
	}
	return parsed, nil
}

func defaultPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if value.Scheme == "https" {
		return "443"
	}
	return "80"
}
