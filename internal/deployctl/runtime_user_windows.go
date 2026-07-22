//go:build windows

package deployctl

func dockerRuntimeUser() string {
	return "picgallery"
}
