package main

import (
	"log"
	"os"

	"github.com/0verL1nk/GraduateProject/desktop/internal/app"
	"github.com/0verL1nk/GraduateProject/desktop/internal/updater"
)

func main() {
	if handled, err := updater.RunUpdateHelper(os.Args[1:]); handled {
		if err != nil {
			log.Printf("desktop: update helper: %v", err)
		}
		return
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
