// Command migrate applies the embedded SQL migrations once, separately from
// starting the request-serving process. Set DATABASE_MIGRATION_URL to a direct
// database connection when the runtime DATABASE_URL uses a transaction pooler.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate: load config: %v\n", err)
		os.Exit(1)
	}
	if err := store.MigrateUp(context.Background(), cfg.MigrationDSN()); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: apply migrations: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("migrate: schema is up to date")
}
