package sqlite

import (
	"path/filepath"
	"testing"
)

func TestMigrationPreservesInactivePerformanceData(t *testing.T) {
	repo, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.db.Exec(`CREATE TABLE performance_baseline(id INTEGER PRIMARY KEY,captured_at TEXT,assets_krw TEXT);
CREATE TABLE cash_flows(id INTEGER PRIMARY KEY,occurred_at TEXT,amount_krw TEXT,memo TEXT);
INSERT INTO performance_baseline VALUES(1,'2026-10-05T11:27:41Z','1000');
INSERT INTO cash_flows VALUES(1,'2026-10-05T11:28:00Z','100','legacy');
INSERT INTO schema_migrations VALUES(10,'2026-10-05T11:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if err = repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"performance_baseline", "cash_flows"} {
		var count int
		if err = repo.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("legacy data removed: %s %d %v", table, count, err)
		}
	}
}
