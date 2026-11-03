//go:build integration

package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func latestMigrationVersion(t *testing.T, dialect string) uint {
	t.Helper()
	versions := embeddedMigrationVersions(t, dialect)
	return versions[len(versions)-1]
}

func schemaTruthTableCount(t *testing.T) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "schema", "tables", "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	return len(files)
}
