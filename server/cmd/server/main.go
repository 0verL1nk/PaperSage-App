// Command server is the standard Go Framework Preset entrypoint used by
// Vercel. Keep process setup shared with cmd/papersage.
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
