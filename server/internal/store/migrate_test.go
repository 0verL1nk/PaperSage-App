package store

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/stub"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/migrations"
)

func embeddedMigrationVersions(t *testing.T, dialect string) []uint {
	t.Helper()
	migrationFS, directory, err := migrations.FSFor(dialect)
	require.NoError(t, err)
	source, err := iofs.New(migrationFS, directory)
	require.NoError(t, err)
	defer func() { require.NoError(t, source.Close()) }()
	current, err := source.First()
	require.NoError(t, err)
	var versions []uint
	for {
		up, _, err := source.ReadUp(current)
		require.NoError(t, err)
		require.NoError(t, up.Close())
		down, _, err := source.ReadDown(current)
		require.NoError(t, err)
		require.NoError(t, down.Close())
		versions = append(versions, current)
		next, err := source.Next(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		require.NoError(t, err)
		current = next
	}
	return versions
}

func TestMigrationsRecognizeDeployedSchema(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql"} {
		t.Run(dialect, func(t *testing.T) {
			versions := embeddedMigrationVersions(t, dialect)
			for _, historical := range []uint{19, 20, 21} {
				require.Contains(t, versions, historical, "deployed migration history must remain packaged")
			}
			head := versions[len(versions)-1]
			for _, version := range []int{18, 21} {
				t.Run(fmt.Sprintf("from_%d", version), func(t *testing.T) {
					migrationFS, directory, err := migrations.FSFor(dialect)
					require.NoError(t, err)
					source, err := iofs.New(migrationFS, directory)
					require.NoError(t, err)
					database := &stub.Stub{CurrentVersion: version}
					runner, err := migrate.NewWithInstance("iofs", source, "stub", database)
					require.NoError(t, err)
					t.Cleanup(func() {
						sourceErr, databaseErr := runner.Close()
						require.NoError(t, sourceErr)
						require.NoError(t, databaseErr)
					})
					expectedSteps := 0
					for _, candidate := range versions {
						if candidate > uint(version) {
							expectedSteps++
						}
					}
					err = runner.Up()
					if expectedSteps == 0 {
						require.ErrorIs(t, err, migrate.ErrNoChange)
					} else {
						require.NoError(t, err)
					}
					require.Equal(t, expectedSteps, len(database.MigrationSequence))
					require.Equal(t, int(head), database.CurrentVersion)
					require.False(t, database.IsDirty)
					require.ErrorIs(t, runner.Up(), migrate.ErrNoChange)
					require.Equal(t, expectedSteps, len(database.MigrationSequence), "current schema must not replay DDL")
				})
			}
		})
	}
}

func TestToDriverDSNAddsTiDBIsolationOverrideOnlyForTiDB(t *testing.T) {
	tidb := toDriverDSN("tidb://root@localhost:4000/papersage")
	if !strings.Contains(tidb, "tidb_skip_isolation_level_check=1") {
		t.Fatalf("TiDB DSN missing isolation override: %s", tidb)
	}
	if strings.Contains(tidb, "?parseTime=true") && !strings.Contains(tidb, "&loc=UTC") {
		t.Fatalf("TiDB DSN missing required UTC parameter: %s", tidb)
	}

	mysql := toDriverDSN("mysql://root@localhost:3306/papersage")
	if strings.Contains(mysql, "tidb_skip_isolation_level_check") {
		t.Fatalf("plain MySQL DSN must not set a TiDB-only variable: %s", mysql)
	}
	if !strings.Contains(mysql, "parseTime=true") || !strings.Contains(mysql, "loc=UTC") || !strings.Contains(mysql, "charset=utf8mb4") {
		t.Fatalf("MySQL DSN is missing required connection parameters: %s", mysql)
	}
}
