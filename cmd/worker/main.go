package main

import (
	"log"
	"os"

	"github.com/fatballfish/pic-gallery/internal/app"
)

func main() {
	if err := app.RunWorker(); err != nil {
		log.Printf("pic-gallery worker exited with error: %v", err)
		os.Exit(1)
	}
}
