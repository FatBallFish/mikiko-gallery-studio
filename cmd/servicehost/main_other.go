//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "mikiko-gallery-studio-service-host is only supported on Windows")
	os.Exit(1)
}
