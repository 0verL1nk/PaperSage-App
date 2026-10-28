// Command server serves the PaperSage API through Vercel's Go Framework Preset.
package main

import (
	"fmt"
	"os"

	"github.com/0verL1nk/GraduateProject/server/internal/serverapp"
)

func main() {
	if err := serverapp.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "papersage: %v\n", err)
		os.Exit(1)
	}
}
