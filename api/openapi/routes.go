package openapi

import (
	_ "embed"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var document []byte

type RouteContract struct {
	routes map[string]map[string]struct{}
}

var (
	routeContractOnce sync.Once
	routeContract     *RouteContract
	routeContractErr  error
)

// LoadRouteContract validates and caches the embedded contract on first normal-router construction.
func LoadRouteContract() (*RouteContract, error) {
	routeContractOnce.Do(func() {
		var routes map[string]map[string]struct{}
		routes, routeContractErr = parseRouteContract(document)
		if routeContractErr == nil {
			routeContract = &RouteContract{routes: routes}
		}
	})
	return routeContract, routeContractErr
}

// Allows reports whether the embedded OpenAPI contract contains the path and method.
func Allows(method, path string) bool {
	contract, err := LoadRouteContract()
	return err == nil && contract.Allows(method, path)
}

func (contract *RouteContract) Allows(method, path string) bool {
	if contract == nil {
		return false
	}
	method = strings.ToLower(strings.TrimSpace(method))
	bestSpecificity := -1
	allowed := false
	for template, methods := range contract.routes {
		if !matchPathTemplate(template, path) {
			continue
		}
		specificity := pathTemplateSpecificity(template)
		if specificity < bestSpecificity {
			continue
		}
		_, methodAllowed := methods[method]
		if specificity > bestSpecificity {
			bestSpecificity = specificity
			allowed = methodAllowed
		} else {
			allowed = allowed || methodAllowed
		}
	}
	return allowed
}

func parseRouteContract(data []byte) (map[string]map[string]struct{}, error) {
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse embedded OpenAPI route contract: %w", err)
	}
	if len(spec.Paths) == 0 {
		return nil, fmt.Errorf("parse embedded OpenAPI route contract: paths are empty")
	}
	contract := make(map[string]map[string]struct{})
	for path, operations := range spec.Paths {
		methods := make(map[string]struct{})
		for method := range operations {
			switch strings.ToUpper(method) {
			case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				methods[strings.ToLower(method)] = struct{}{}
			}
		}
		if len(methods) > 0 {
			contract[path] = methods
		}
	}
	if len(contract) == 0 {
		return nil, fmt.Errorf("parse embedded OpenAPI route contract: operations are empty")
	}
	return contract, nil
}

func matchPathTemplate(template, path string) bool {
	templateSegments, templateOK := splitAbsolutePath(template)
	pathSegments, pathOK := splitAbsolutePath(path)
	if !templateOK || !pathOK {
		return false
	}
	if len(templateSegments) != len(pathSegments) {
		return false
	}
	for i, segment := range templateSegments {
		open := strings.IndexByte(segment, '{')
		close := strings.IndexByte(segment, '}')
		if open < 0 || close < open {
			if segment != pathSegments[i] {
				return false
			}
			continue
		}
		prefix, suffix := segment[:open], segment[close+1:]
		value := pathSegments[i]
		if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) || len(value) <= len(prefix)+len(suffix) {
			return false
		}
	}
	return true
}

func splitAbsolutePath(path string) ([]string, bool) {
	if path == "/" {
		return nil, true
	}
	if !strings.HasPrefix(path, "/") || path == "" {
		return nil, false
	}
	return strings.Split(strings.TrimPrefix(path, "/"), "/"), true
}

func pathTemplateSpecificity(template string) int {
	specificity := 0
	inParameter := false
	for _, char := range template {
		switch char {
		case '{':
			inParameter = true
		case '}':
			inParameter = false
		default:
			if !inParameter {
				specificity++
			}
		}
	}
	return specificity
}
