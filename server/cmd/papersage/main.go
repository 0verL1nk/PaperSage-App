// Command papersage serves the PaperSage API and, when frontend assets are
// embedded, the SPA from a single binary.
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
