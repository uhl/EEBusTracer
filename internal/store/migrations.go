package store

import (
	"database/sql"
	"fmt"
)

func migrate(db *sql.DB) error {
	// Create schema_version table if it doesn't exist
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	var version int
	err := db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		version = 0
		if _, err := db.Exec("INSERT INTO schema_version (version) VALUES (0)"); err != nil {
			return fmt.Errorf("init schema version: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if version < 1 {
		if err := migrateV1(db); err != nil {
			return fmt.Errorf("migrate v1: %w", err)
		}
	}

	if version < 2 {
		if err := migrateV2(db); err != nil {
			return fmt.Errorf("migrate v2: %w", err)
		}
	}

	if version < 3 {
		if err := migrateV3(db); err != nil {
			return fmt.Errorf("migrate v3: %w", err)
		}
	}

	if version < 4 {
		if err := migrateV4(db); err != nil {
			return fmt.Errorf("migrate v4: %w", err)
		}
	}

	if version < 5 {
		if err := migrateV5(db); err != nil {
			return fmt.Errorf("migrate v5: %w", err)
		}
	}

	return nil
}

func migrateV5(db *sql.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_messages_msg_counter ON messages(trace_id, msg_counter)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_msg_counter_ref ON messages(trace_id, msg_counter_ref)`,
		`UPDATE schema_version SET version = 5`,
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("execute %q: %w", stmt[:min(40, len(stmt))], err)
		}
	}

	return tx.Commit()
}

func migrateV4(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS chart_definitions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			trace_id INTEGER,
			chart_type TEXT NOT NULL DEFAULT 'line',
			is_built_in BOOLEAN NOT NULL DEFAULT 0,
			sources TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (trace_id) REFERENCES traces(id) ON DELETE CASCADE
		)`,

		// Seed built-in: Measurements
		`INSERT INTO chart_definitions (name, chart_type, is_built_in, sources) VALUES (
			'Measurements', 'line', 1,
			'[{"functionSet":"MeasurementListData","cmdKey":"measurementListData","dataArrayKey":"measurementData","idField":"measurementId","classifiers":["reply","notify"]}]'
		)`,

		// Seed built-in: Load Control
		`INSERT INTO chart_definitions (name, chart_type, is_built_in, sources) VALUES (
			'Load Control', 'step', 1,
			'[{"functionSet":"LoadControlLimitListData","cmdKey":"loadControlLimitListData","dataArrayKey":"loadControlLimitData","idField":"limitId","classifiers":["reply","notify","write"]}]'
		)`,

		// Seed built-in: Setpoints
		`INSERT INTO chart_definitions (name, chart_type, is_built_in, sources) VALUES (
			'Setpoints', 'line', 1,
			'[{"functionSet":"SetpointListData","cmdKey":"setpointListData","dataArrayKey":"setpointData","idField":"setpointId","classifiers":["reply","notify"]}]'
		)`,

		`UPDATE schema_version SET version = 4`,
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("execute %q: %w", stmt[:min(40, len(stmt))], err)
		}
	}

	return tx.Commit()
}

func migrateV3(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS mdns_devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			instance_name TEXT NOT NULL UNIQUE,
			host_name TEXT NOT NULL DEFAULT '',
			addresses TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			ski TEXT NOT NULL DEFAULT '',
			brand TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			device_type TEXT NOT NULL DEFAULT '',
			identifier TEXT NOT NULL DEFAULT '',
			first_seen_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL,
			online BOOLEAN NOT NULL DEFAULT 1
		)`,

		`UPDATE schema_version SET version = 3`,
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("execute %q: %w", stmt[:min(40, len(stmt))], err)
		}
	}

	return tx.Commit()
}

func migrateV1(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS traces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			started_at DATETIME NOT NULL,
			stopped_at DATETIME,
			message_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			trace_id INTEGER NOT NULL,
			sequence_num INTEGER NOT NULL,
			timestamp DATETIME NOT NULL,
			direction TEXT NOT NULL DEFAULT 'unknown',
			source_addr TEXT NOT NULL DEFAULT '',
			dest_addr TEXT NOT NULL DEFAULT '',
			raw_hex TEXT NOT NULL DEFAULT '',
			normalized_json TEXT,
			ship_msg_type TEXT NOT NULL DEFAULT 'unknown',
			ship_payload TEXT,
			spine_payload TEXT,
			cmd_classifier TEXT NOT NULL DEFAULT '',
			function_set TEXT NOT NULL DEFAULT '',
			msg_counter TEXT NOT NULL DEFAULT '',
			msg_counter_ref TEXT NOT NULL DEFAULT '',
			device_source TEXT NOT NULL DEFAULT '',
			device_dest TEXT NOT NULL DEFAULT '',
			entity_source TEXT NOT NULL DEFAULT '',
			entity_dest TEXT NOT NULL DEFAULT '',
			feature_source TEXT NOT NULL DEFAULT '',
			feature_dest TEXT NOT NULL DEFAULT '',
			parse_error TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (trace_id) REFERENCES traces(id) ON DELETE CASCADE
		)`,

		`CREATE INDEX IF NOT EXISTS idx_messages_trace_id ON messages(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(trace_id, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_function_set ON messages(trace_id, function_set)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_cmd_classifier ON messages(trace_id, cmd_classifier)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_sequence ON messages(trace_id, sequence_num)`,

		`CREATE TABLE IF NOT EXISTS devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			trace_id INTEGER NOT NULL,
			device_addr TEXT NOT NULL,
			ski TEXT NOT NULL DEFAULT '',
			brand TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			device_type TEXT NOT NULL DEFAULT '',
			first_seen_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL,
			FOREIGN KEY (trace_id) REFERENCES traces(id) ON DELETE CASCADE,
			UNIQUE(trace_id, device_addr)
		)`,

		`UPDATE schema_version SET version = 1`,
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("execute %q: %w", stmt[:40], err)
		}
	}

	return tx.Commit()
}

func migrateV2(db *sql.DB) error {
	stmts := []string{
		// Full-text search on message content
		`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
			normalized_json, ship_payload, spine_payload, parse_error,
			content=messages, content_rowid=id
		)`,

		// FTS trigger: keep index in sync on INSERT
		`CREATE TRIGGER IF NOT EXISTS messages_fts_insert AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, normalized_json, ship_payload, spine_payload, parse_error)
			VALUES (new.id, new.normalized_json, new.ship_payload, new.spine_payload, new.parse_error);
		END`,

		// FTS trigger: keep index in sync on DELETE
		`CREATE TRIGGER IF NOT EXISTS messages_fts_delete AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, normalized_json, ship_payload, spine_payload, parse_error)
			VALUES ('delete', old.id, old.normalized_json, old.ship_payload, old.spine_payload, old.parse_error);
		END`,

		// FTS trigger: keep index in sync on UPDATE
		`CREATE TRIGGER IF NOT EXISTS messages_fts_update AFTER UPDATE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, normalized_json, ship_payload, spine_payload, parse_error)
			VALUES ('delete', old.id, old.normalized_json, old.ship_payload, old.spine_payload, old.parse_error);
			INSERT INTO messages_fts(rowid, normalized_json, ship_payload, spine_payload, parse_error)
			VALUES (new.id, new.normalized_json, new.ship_payload, new.spine_payload, new.parse_error);
		END`,

		// Bookmarks
		`CREATE TABLE IF NOT EXISTS bookmarks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id INTEGER NOT NULL,
			trace_id INTEGER NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			color TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
			FOREIGN KEY (trace_id) REFERENCES traces(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_bookmarks_trace ON bookmarks(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_bookmarks_message ON bookmarks(message_id)`,

		// Filter presets
		`CREATE TABLE IF NOT EXISTS filter_presets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			filter_json TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`UPDATE schema_version SET version = 2`,
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("execute %q: %w", stmt[:min(40, len(stmt))], err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit v2 migration: %w", err)
	}

	// Backfill FTS index with existing messages (outside transaction since FTS operations
	// on virtual tables can conflict with transactions on some SQLite builds)
	if _, err := db.Exec(`INSERT INTO messages_fts(rowid, normalized_json, ship_payload, spine_payload, parse_error)
		SELECT id, normalized_json, ship_payload, spine_payload, parse_error FROM messages`); err != nil {
		return fmt.Errorf("backfill FTS index: %w", err)
	}

	return nil
}
