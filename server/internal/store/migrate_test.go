package store

import (
	"strings"
	"testing"
)

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
