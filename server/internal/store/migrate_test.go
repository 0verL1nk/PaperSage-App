package store

import (
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/stub"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/migrations"
)

func TestMigrationsRecognizeDeployedSchema(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql"} {
		t.Run(dialect, func(t *testing.T) {
			for _, version := range []int{18, 21} {
				name := "upgrade_from_18"
				if version == 21 {
					name = "already_at_21"
				}
				t.Run(name, func(t *testing.T) {
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

					err = runner.Up()
					if version == 21 {
						require.ErrorIs(t, err, migrate.ErrNoChange)
						require.Empty(t, database.MigrationSequence, "an already deployed schema must not replay DDL")
					} else {
						require.NoError(t, err)
						require.Len(t, database.MigrationSequence, 3, "restore every migration between schema 18 and 21")
					}
					require.Equal(t, 21, database.CurrentVersion)
					require.False(t, database.IsDirty)
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
