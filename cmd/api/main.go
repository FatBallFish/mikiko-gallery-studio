package main

import (
	"log"
	"os"

	"github.com/fatballfish/pic-gallery/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		code := app.ExitCode(err)
		if code != app.SupervisorRestartExitCode {
			log.Printf("pic-gallery api exited with error: %v", err)
		}
		os.Exit(code)
	}
}
