package mgsctl

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
	Dirty     bool
}

type buildInfoDocument struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	Dirty     bool   `json:"dirty"`
	GoVersion string `json:"go_version"`
	GoOS      string `json:"go_os"`
	GoArch    string `json:"go_arch"`
}

func NormalizeBuildInfo(info BuildInfo) BuildInfo {
	info.Version = defaultBuildValue(info.Version, "dev")
	info.Commit = defaultBuildValue(info.Commit, "unknown")
	info.BuildTime = defaultBuildValue(info.BuildTime, "unknown")
	return info
}

func (info BuildInfo) Text() string {
	info = NormalizeBuildInfo(info)
	return fmt.Sprintf(
		"mgsctl %s\ncommit: %s\nbuilt: %s\ndirty: %t\ngo: %s\nplatform: %s/%s",
		info.Version, info.Commit, info.BuildTime, info.Dirty, runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
}

func (info BuildInfo) JSON() ([]byte, error) {
	info = NormalizeBuildInfo(info)
	return json.Marshal(buildInfoDocument{
		Version: info.Version, Commit: info.Commit, BuildTime: info.BuildTime, Dirty: info.Dirty,
		GoVersion: runtime.Version(), GoOS: runtime.GOOS, GoArch: runtime.GOARCH,
	})
}

func defaultBuildValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
