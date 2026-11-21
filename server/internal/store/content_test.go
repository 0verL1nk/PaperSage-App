package store_test

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/migrations"
)

// Exercise the actual SQLite AutoMigrate path, not just the model tags.
func TestContentConstraints(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/content.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	user, project, source, version, extraction := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: user}, Username: "content", PasswordHash: "x"}).Error)
	require.NoError(t, db.Create(&store.Project{BaseModel: store.BaseModel{ID: project}, Name: "p", OwnerID: user}).Error)
	exec := func(query string, args ...any) error { return db.Exec(query, args...).Error }
	require.NoError(t, exec("INSERT INTO project_sources (id, project_id, kind, created_at, updated_at) VALUES (?, ?, 'document', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", source, project))
	var defaults struct {
		CurrentRevision int64
		State           string
	}
	require.NoError(t, db.Raw("SELECT current_revision, state FROM project_sources WHERE id = ?", source).Scan(&defaults).Error)
	require.EqualValues(t, 0, defaults.CurrentRevision)
	require.Equal(t, "active", defaults.State)

	versionSQL := "INSERT INTO source_versions (id, source_id, revision, s3_key, content_sha256, size_bytes, created_at) VALUES (?, ?, ?, 'key', 'hash', ?, CURRENT_TIMESTAMP)"
	require.Error(t, exec(versionSQL, store.MustNewUUID(), source, 0, 1))
	require.Error(t, exec(versionSQL, store.MustNewUUID(), source, 1, -1))
	require.Error(t, exec(versionSQL, store.MustNewUUID(), store.MustNewUUID(), 1, 1))
	require.NoError(t, exec(versionSQL, version, source, 1, 1))
	require.Error(t, exec(versionSQL, store.MustNewUUID(), source, 1, 1))
	extractSQL := "INSERT INTO source_extractions (id, source_version_id, extractor_version, created_at, updated_at) VALUES (?, ?, 'v1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"
	require.NoError(t, exec(extractSQL, extraction, version))
	require.Error(t, exec(extractSQL, store.MustNewUUID(), version))
	var extractionDefaults struct {
		State        string
		TextBytes    int64
		AttemptCount int64
	}
	require.NoError(t, db.Raw("SELECT state, text_bytes, attempt_count FROM source_extractions WHERE id = ?", extraction).Scan(&extractionDefaults).Error)
	require.Equal(t, "pending", extractionDefaults.State)
	require.Zero(t, extractionDefaults.TextBytes)
	require.Zero(t, extractionDefaults.AttemptCount)
	segmentSQL := "INSERT INTO source_segments (id, extraction_id, ordinal, start_byte, end_byte, locator_kind, physical_page) VALUES (?, ?, ?, ?, ?, 'physical_page', ?)"
	require.NoError(t, exec(segmentSQL, store.MustNewUUID(), extraction, 0, 0, 10, 1))
	require.Error(t, exec(segmentSQL, store.MustNewUUID(), extraction, 0, 0, 10, 1))
	require.Error(t, exec(segmentSQL, store.MustNewUUID(), extraction, -1, 0, 10, 1))
	require.Error(t, exec(segmentSQL, store.MustNewUUID(), extraction, 1, -1, 10, 1))
	require.Error(t, exec(segmentSQL, store.MustNewUUID(), extraction, 1, 10, 9, 1))
	require.Error(t, exec(segmentSQL, store.MustNewUUID(), extraction, 1, 0, 10, 0))
	require.Error(t, exec("DELETE FROM project_sources WHERE id = ?", source))
	// Self-references also protect history until the dependent version is removed.
	creator := store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: creator}, Username: "creator", PasswordHash: "x"}).Error)
	original := store.MustNewUUID()
	require.NoError(t, exec(versionSQL, original, source, 2, 1))
	restored := store.MustNewUUID()
	require.NoError(t, exec("INSERT INTO source_versions (id, source_id, revision, s3_key, content_sha256, size_bytes, restored_from_id, created_by, created_at) VALUES (?, ?, 3, 'key', 'hash', 1, ?, ?, CURRENT_TIMESTAMP)", restored, source, original, creator))
	require.Error(t, exec("DELETE FROM source_versions WHERE id = ?", original))
	require.NoError(t, exec("DELETE FROM users WHERE id = ?", creator))
	var createdBy *string
	require.NoError(t, db.Raw("SELECT created_by FROM source_versions WHERE id = ?", restored).Row().Scan(&createdBy))
	require.Nil(t, createdBy)
}

func TestSourceAssetMapping(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/assets.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))
	user, project, source := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: user}, Username: "asset", PasswordHash: "x"}).Error)
	require.NoError(t, db.Create(&store.Project{BaseModel: store.BaseModel{ID: project}, Name: "p", OwnerID: user}).Error)
	require.NoError(t, db.Exec("INSERT INTO project_sources (id, project_id, kind, created_at, updated_at) VALUES (?, ?, 'document', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", source, project).Error)
	insertDoc := func(id store.UUID, src any) error {
		return db.Exec("INSERT INTO documents (id, name, type, size, status, s3_key, project_id, owner_id, source_id, created_at, updated_at) VALUES (?, 'd', 'pdf', 0, 'uploaded', 'key', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", id, project, user, src).Error
	}
	insertLatex := func(id store.UUID, path string, src any) error {
		return db.Exec("INSERT INTO latex_files (id, project_id, file_path, source_id, created_at, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", id, project, path, src).Error
	}
	require.NoError(t, insertDoc(store.MustNewUUID(), nil))
	require.NoError(t, insertDoc(store.MustNewUUID(), nil))
	require.NoError(t, insertDoc(store.MustNewUUID(), source))
	require.Error(t, insertDoc(store.MustNewUUID(), source))
	require.Error(t, insertDoc(store.MustNewUUID(), store.MustNewUUID()))
	require.NoError(t, insertLatex(store.MustNewUUID(), "a", nil))
	require.NoError(t, insertLatex(store.MustNewUUID(), "b", nil))
	require.NoError(t, insertLatex(store.MustNewUUID(), "c", source))
	require.Error(t, insertLatex(store.MustNewUUID(), "d", source))
	require.Error(t, insertLatex(store.MustNewUUID(), "e", store.MustNewUUID()))
	require.Error(t, db.Exec("DELETE FROM project_sources WHERE id = ?", source).Error)
}

// Both INSERT and UPDATE must reject an existing source from another project.
func TestSourceAssetProjectIsolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/isolation.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, db.AutoMigrate(store.AllModels()...))

	owner, p1, p2 := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
	s1, s2 := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: owner}, Username: "isolation", PasswordHash: "x"}).Error)
	for _, p := range []store.UUID{p1, p2} {
		require.NoError(t, db.Create(&store.Project{BaseModel: store.BaseModel{ID: p}, Name: p.String(), OwnerID: owner}).Error)
	}
	for _, item := range []struct{ source, project store.UUID }{{s1, p1}, {s2, p2}} {
		require.NoError(t, db.Exec("INSERT INTO project_sources (id, project_id, kind, created_at, updated_at) VALUES (?, ?, 'document', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", item.source, item.project).Error)
	}
	for _, tc := range []struct {
		name, insert string
	}{
		{"documents", "INSERT INTO documents (id, name, type, size, status, s3_key, project_id, owner_id, source_id, created_at, updated_at) VALUES (?, 'd', 'pdf', 0, 'uploaded', 'key', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"},
		{"latex_files", "INSERT INTO latex_files (id, file_path, project_id, source_id, created_at, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			insert := func(id, project store.UUID, source any) error {
				if tc.name == "documents" {
					return db.Exec(tc.insert, id, project, owner, source).Error
				}
				return db.Exec(tc.insert, id, id.String(), project, source).Error
			}
			require.Error(t, insert(store.MustNewUUID(), p1, s2), "cross-project INSERT")
			id := store.MustNewUUID()
			require.NoError(t, insert(id, p1, nil), "legacy NULL source")
			require.Error(t, db.Exec("UPDATE "+tc.name+" SET source_id = ? WHERE id = ?", s2, id).Error, "cross-project UPDATE")
			require.NoError(t, db.Exec("UPDATE "+tc.name+" SET source_id = ? WHERE id = ?", s1, id).Error, "same-project association")
			require.Error(t, db.Exec("UPDATE "+tc.name+" SET project_id = ? WHERE id = ?", p2, id).Error, "moving an associated asset across projects")
			require.NoError(t, db.Exec("UPDATE "+tc.name+" SET source_id = NULL WHERE id = ?", id).Error)
		})
	}
}

// Opt in only on disposable local servers: these tests create and drop an isolated schema/database.
func TestContentMigrationDialectIsolation(t *testing.T) {
	if os.Getenv("PAPERSAGE_RUN_CONTENT_INTEGRATION") != "1" {
		t.Skip("requires explicit opt-in and disposable PostgreSQL/MySQL servers")
	}
	for _, tc := range []struct{ dialect, driver, env, idType string }{
		{"postgres", "pgx/v5", "PAPERSAGE_TEST_CONTENT_PG_DSN", "uuid"},
		{"mysql", "mysql", "PAPERSAGE_TEST_CONTENT_MYSQL_DSN", "char(32)"},
	} {
		t.Run(tc.dialect, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skip("set " + tc.env + " to a disposable local server")
			}
			db, err := sql.Open(tc.driver, dsn)
			require.NoError(t, err)
			defer db.Close()
			db.SetMaxOpenConns(1)
			name := "content_test_" + strings.ReplaceAll(store.MustNewUUID().String(), "-", "")
			if tc.dialect == "postgres" {
				_, err = db.Exec("CREATE SCHEMA " + name)
				require.NoError(t, err)
				defer func() { _, _ = db.Exec("DROP SCHEMA " + name + " CASCADE") }()
				_, err = db.Exec("SET search_path TO " + name)
			} else {
				_, err = db.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_bin")
				require.NoError(t, err)
				defer func() { _, _ = db.Exec("DROP DATABASE " + name) }()
				_, err = db.Exec("USE " + name)
			}
			require.NoError(t, err)
			for _, table := range []string{"projects", "users"} {
				_, err = db.Exec("CREATE TABLE " + table + " (id " + tc.idType + " PRIMARY KEY)")
				require.NoError(t, err)
			}
			for _, table := range []string{"documents", "latex_files"} {
				_, err = db.Exec("CREATE TABLE " + table + " (id " + tc.idType + " PRIMARY KEY, project_id " + tc.idType + " NOT NULL)")
				require.NoError(t, err)
			}
			fs, dir, err := migrations.FSFor(tc.dialect)
			require.NoError(t, err)
			migrate := func(direction string) {
				body, readErr := fs.ReadFile(path.Join(dir, "0022_versioned_content."+direction+".sql"))
				require.NoError(t, readErr)
				for _, statement := range strings.Split(string(body), ";") {
					if strings.TrimSpace(statement) != "" {
						_, execErr := db.Exec(statement)
						require.NoError(t, execErr, "%s migration statement: %s", direction, statement)
					}
				}
			}
			migrate("up")
			exec := func(query string, args ...any) error {
				if tc.dialect == "postgres" {
					for i := range args {
						query = strings.Replace(query, "?", fmt.Sprintf("$%d", i+1), 1)
					}
				}
				_, execErr := db.Exec(query, args...)
				return execErr
			}
			id := func() string { return strings.ReplaceAll(store.MustNewUUID().String(), "-", "") }
			p1, p2, s1, s2 := id(), id(), id(), id()
			require.NoError(t, exec("INSERT INTO projects (id) VALUES (?), (?)", p1, p2))
			require.NoError(t, exec("INSERT INTO project_sources (id, project_id, kind) VALUES (?, ?, 'document'), (?, ?, 'document')", s1, p1, s2, p2))
			for _, table := range []string{"documents", "latex_files"} {
				require.Error(t, exec("INSERT INTO "+table+" (id, project_id, source_id) VALUES (?, ?, ?)", id(), p1, s2), "cross-project INSERT: %s", table)
				asset := id()
				require.NoError(t, exec("INSERT INTO "+table+" (id, project_id, source_id) VALUES (?, ?, NULL), (?, ?, NULL)", asset, p1, id(), p1))
				require.Error(t, exec("UPDATE "+table+" SET source_id = ? WHERE id = ?", s2, asset), "cross-project UPDATE: %s", table)
				require.NoError(t, exec("UPDATE "+table+" SET source_id = ? WHERE id = ?", s1, asset))
				require.Error(t, exec("UPDATE "+table+" SET project_id = ? WHERE id = ?", p2, asset), "cross-project move: %s", table)
			}
			migrate("down")
		})
	}
}
