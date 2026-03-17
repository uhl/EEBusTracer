package store

import (
	"database/sql"
	"fmt"

	"github.com/eebustracer/eebustracer/internal/model"
)

// PresetRepo provides CRUD operations for filter presets.
type PresetRepo struct {
	db *sql.DB
}

// NewPresetRepo creates a new PresetRepo.
func NewPresetRepo(db *DB) *PresetRepo {
	return &PresetRepo{db: db.SqlDB()}
}

// Create inserts a new filter preset.
func (r *PresetRepo) Create(p *model.FilterPreset) error {
	result, err := r.db.Exec(
		`INSERT INTO filter_presets (name, filter_json, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		p.Name, p.Filter,
	)
	if err != nil {
		return fmt.Errorf("insert preset: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	p.ID = id
	return nil
}

// List returns all filter presets.
func (r *PresetRepo) List() ([]*model.FilterPreset, error) {
	rows, err := r.db.Query(
		`SELECT id, name, filter_json, created_at FROM filter_presets ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list presets: %w", err)
	}
	defer rows.Close()

	var presets []*model.FilterPreset
	for rows.Next() {
		p := &model.FilterPreset{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Filter, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan preset: %w", err)
		}
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

// Get retrieves a preset by ID.
func (r *PresetRepo) Get(id int64) (*model.FilterPreset, error) {
	p := &model.FilterPreset{}
	err := r.db.QueryRow(
		`SELECT id, name, filter_json, created_at FROM filter_presets WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Filter, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get preset: %w", err)
	}
	return p, nil
}

// Delete removes a preset by ID.
func (r *PresetRepo) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM filter_presets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete preset: %w", err)
	}
	return nil
}
