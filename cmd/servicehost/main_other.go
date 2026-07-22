//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "pic-gallery-service-host is only supported on Windows")
	os.Exit(1)
}
