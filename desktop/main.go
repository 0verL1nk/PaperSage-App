package main

import (
	"log"

	"github.com/0verL1nk/GraduateProject/desktop/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
