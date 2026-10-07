package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 用 sqlmock 驱动交接探测与 stamp 的三条路径（runMigrations 需真实 PG，
// 由 CI 集成与 E2E 覆盖）。

const tableExistsSQL = "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)"

func newEqualMock() (*sqlDB, sqlmock.Sqlmock, error) {
	return sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
}

// sqlDB 别名让辅助函数签名简洁
type sqlDB = sql.DB

func expectTableExists(dbmock sqlmock.Sqlmock, table string, exists bool) {
	dbmock.ExpectQuery(tableExistsSQL).
		WithArgs(table).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func TestProbeStateFresh(t *testing.T) {
	db, mock, _ := newEqualMock()
	defer func() { _ = db.Close() }()

	expectTableExists(mock, "alembic_version", false)
	expectTableExists(mock, "schema_migrations", false)
	expectTableExists(mock, "users", false)

	st, err := probeState(context.Background(), db, "postgres")
	require.NoError(t, err)
	assert.Equal(t, state{}, st)
	assert.Equal(t, actionFreshInstall, decideHandover(st))
}

func TestProbeStateLegacyDB(t *testing.T) {
	db, mock, _ := newEqualMock()
	defer func() { _ = db.Close() }()

	expectTableExists(mock, "alembic_version", true)
	expectTableExists(mock, "schema_migrations", false)
	expectTableExists(mock, "users", true)

	st, err := probeState(context.Background(), db, "postgres")
	require.NoError(t, err)
	assert.True(t, st.HasAlembic)
	assert.True(t, st.HasUsers)
	assert.Equal(t, actionStampBaseline, decideHandover(st))
}

func TestStampBaselineSQL(t *testing.T) {
	db, mock, _ := newEqualMock()
	defer func() { _ = db.Close() }()

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations (version bigint not null primary key, dirty boolean not null)").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO schema_migrations (version, dirty) VALUES ($1, false) ON CONFLICT (version) DO NOTHING").
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, stampBaseline(context.Background(), db, "postgres"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProbeStateQueryError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`SELECT EXISTS`).WithArgs("alembic_version").
		WillReturnError(assert.AnError)

	_, err := probeState(context.Background(), db, "postgres")
	assert.Error(t, err)
}
