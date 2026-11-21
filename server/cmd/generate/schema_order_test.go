package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderSchemaFilesRespectsRepositoryForeignKeys(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", tablesDir, "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	ordered, err := orderSchemaFiles(files)
	require.NoError(t, err)
	positions := map[string]int{}
	for i, name := range ordered {
		positions[filepath.Base(name)] = i
	}
	for _, pair := range [][2]string{
		{"users.sql", "projects.sql"},
		{"projects.sql", "project_sources.sql"},
		{"project_sources.sql", "documents.sql"},
		{"project_sources.sql", "latex_files.sql"},
		{"project_sources.sql", "source_versions.sql"},
		{"source_versions.sql", "source_extractions.sql"},
		{"source_extractions.sql", "source_segments.sql"},
		{"agent_runs.sql", "agent_artifacts.sql"},
	} {
		require.Contains(t, positions, pair[0])
		require.Contains(t, positions, pair[1])
		require.Less(t, positions[pair[0]], positions[pair[1]], "%s must exist before %s", pair[0], pair[1])
	}
	slices.Reverse(files)
	again, err := orderSchemaFiles(files)
	require.NoError(t, err)
	require.Equal(t, ordered, again, "independent input order must not change generation")
}

func TestOrderSchemaFilesHandlesSelfReferenceAndRejectsInvalidDependencies(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ddl   map[string]string
		valid bool
	}{
		{name: "self reference", ddl: map[string]string{"nodes": "CREATE TABLE nodes (parent_id int REFERENCES `nodes` (id));"}, valid: true},
		{name: "missing parent", ddl: map[string]string{"child": "CREATE TABLE child (parent_id int REFERENCES absent (id));"}},
		{name: "cycle", ddl: map[string]string{"a": "CREATE TABLE a (b_id int REFERENCES `b` (id));", "b": "CREATE TABLE b (a_id int REFERENCES a (id));"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			var files []string
			for table, ddl := range tc.ddl {
				name := filepath.Join(directory, table+".sql")
				require.NoError(t, os.WriteFile(name, []byte(ddl), 0o600))
				files = append(files, name)
			}
			ordered, err := orderSchemaFiles(files)
			if tc.valid {
				require.NoError(t, err)
				require.Len(t, ordered, len(files))
			} else {
				require.Error(t, err)
				require.Empty(t, ordered)
			}
		})
	}
}
