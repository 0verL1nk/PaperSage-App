package main

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestInitSchemaPreservesExistingChunks(t *testing.T) {
	rows := 3
	destructive := regexp.MustCompile(`(?i)\b(DROP\s+TABLE|TRUNCATE|DELETE\s+FROM)\b`)
	var statements []string
	err := initSchema(func(statement string) error {
		statements = append(statements, statement)
		if destructive.MatchString(statement) {
			rows = 0
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("initialization would remove existing chunks; statements: %q", statements)
	}
	if !strings.Contains(strings.Join(statements, "\n"), "CREATE TABLE IF NOT EXISTS rag_chunks") {
		t.Fatal("initialization must still create the table when absent")
	}
}

func TestInitSchemaReportsMissingColumn(t *testing.T) {
	incompatible := errors.New("column generation does not exist")
	var statements []string
	err := initSchema(func(statement string) error {
		statements = append(statements, statement)
		if strings.HasPrefix(statement, "SELECT id, project_id, source_type, source_id, source_name, chunk_idx, content, embedding, generation FROM rag_chunks WHERE false") {
			return incompatible
		}
		return nil
	})
	if !errors.Is(err, incompatible) {
		t.Fatalf("expected incompatible schema error, got %v", err)
	}
	if strings.Contains(strings.Join(statements, "\n"), "DROP TABLE") {
		t.Fatal("incompatible tables must not be dropped")
	}
}
