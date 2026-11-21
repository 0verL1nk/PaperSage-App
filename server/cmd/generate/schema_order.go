package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// shortcut: repository DDL uses unqualified table names; use a SQL parser if that syntax expands.
var schemaReference = regexp.MustCompile("(?i)\\bREFERENCES\\s+`?([a-z_][a-z0-9_]*)`?")

func orderSchemaFiles(files []string) ([]string, error) {
	paths := make(map[string]string, len(files))
	dependencies := make(map[string][]string, len(files))
	names := make([]string, 0, len(files))
	for _, file := range files {
		name := strings.ToLower(strings.TrimSuffix(filepath.Base(file), ".sql"))
		if _, exists := paths[name]; exists {
			return nil, fmt.Errorf("duplicate schema table %s", name)
		}
		ddl, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", file, err)
		}
		paths[name] = file
		names = append(names, name)
		for _, match := range schemaReference.FindAllSubmatch(ddl, -1) {
			parent := strings.ToLower(string(match[1]))
			if parent != name {
				dependencies[name] = append(dependencies[name], parent)
			}
		}
	}
	slices.Sort(names)
	const (
		visiting = 1
		visited  = 2
	)
	states := make(map[string]int, len(files))
	ordered := make([]string, 0, len(files))
	var visit func(string) error
	visit = func(name string) error {
		switch states[name] {
		case visiting:
			return fmt.Errorf("foreign-key dependency cycle at %s", name)
		case visited:
			return nil
		}
		states[name] = visiting
		for _, parent := range dependencies[name] {
			if _, exists := paths[parent]; !exists {
				return fmt.Errorf("schema table %s references missing table %s", name, parent)
			}
			if err := visit(parent); err != nil {
				return err
			}
		}
		states[name] = visited
		ordered = append(ordered, paths[name])
		return nil
	}
	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
