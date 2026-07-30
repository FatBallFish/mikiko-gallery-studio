//go:build windows

package mgsctl

func dockerRuntimeUser() string {
	return "picgallery"
}
