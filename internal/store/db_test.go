package store

import (
	"testing"
	"time"

	"github.com/eebustracer/eebustracer/internal/model"
)

func TestOpen_Memory(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	defer db.Close()
}

func TestMigrate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify tables exist
	tables := []string{"traces", "messages", "devices", "schema_version", "bookmarks", "filter_presets"}
	for _, table := range tables {
		var name string
		err := db.SqlDB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	// Verify FTS virtual table exists
	var ftsName string
	err = db.SqlDB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='messages_fts'").Scan(&ftsName)
	if err != nil {
		t.Errorf("FTS table not found: %v", err)
	}

	// Verify chart_definitions table exists
	var chartTable string
	err = db.SqlDB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='chart_definitions'").Scan(&chartTable)
	if err != nil {
		t.Errorf("chart_definitions table not found: %v", err)
	}

	// Verify schema version is 4
	var version int
	err = db.SqlDB().QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 4 {
		t.Errorf("schema version = %d, want 4", version)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
}

func TestMigrateV1ToV2(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Manually run only v1 migration
	if _, err := db.SqlDB().Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if _, err := db.SqlDB().Exec("INSERT INTO schema_version (version) VALUES (0)"); err != nil {
		t.Fatalf("init version: %v", err)
	}
	if err := migrateV1(db.SqlDB()); err != nil {
		t.Fatalf("migrateV1 failed: %v", err)
	}

	// Insert a message before v2 migration to test backfill
	traceRepo := NewTraceRepo(db)
	trace := &model.Trace{Name: "Test", StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := traceRepo.CreateTrace(trace); err != nil {
		t.Fatalf("CreateTrace: %v", err)
	}
	_, err = db.SqlDB().Exec(
		`INSERT INTO messages (trace_id, sequence_num, timestamp, ship_msg_type, normalized_json)
		 VALUES (?, 1, ?, 'data', '{"test":"backfill_data"}')`,
		trace.ID, time.Now(),
	)
	if err != nil {
		t.Fatalf("insert pre-migration message: %v", err)
	}

	// Now run full migrate which should apply v2
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate (v1→v2) failed: %v", err)
	}

	// Verify FTS table was created and backfilled
	var count int
	err = db.SqlDB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'backfill_data'").Scan(&count)
	if err != nil {
		t.Fatalf("FTS query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("FTS backfill count = %d, want 1", count)
	}
}

func TestFTSTrigger_InsertMessage(t *testing.T) {
	db := newTestDB(t)

	traceRepo := NewTraceRepo(db)
	trace := &model.Trace{Name: "Test", StartedAt: time.Now(), CreatedAt: time.Now()}
	if err := traceRepo.CreateTrace(trace); err != nil {
		t.Fatalf("CreateTrace: %v", err)
	}

	msgRepo := NewMessageRepo(db)
	msg := &model.Message{
		TraceID:       trace.ID,
		SequenceNum:   1,
		Timestamp:     time.Now(),
		ShipMsgType:   model.ShipMsgTypeData,
		SpinePayload:  []byte(`{"functionSet":"MeasurementListData"}`),
	}
	if err := msgRepo.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}

	// Search via FTS
	var count int
	err := db.SqlDB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'MeasurementListData'").Scan(&count)
	if err != nil {
		t.Fatalf("FTS query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("FTS match count = %d, want 1", count)
	}
}

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
