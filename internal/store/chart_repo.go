package store

import (
	"database/sql"
	"fmt"

	"github.com/eebustracer/eebustracer/internal/model"
)

// ChartRepo provides CRUD operations for chart definitions.
type ChartRepo struct {
	db *sql.DB
}

// NewChartRepo creates a new ChartRepo.
func NewChartRepo(db *DB) *ChartRepo {
	return &ChartRepo{db: db.SqlDB()}
}

// Create inserts a new chart definition.
func (r *ChartRepo) Create(cd *model.ChartDefinition) error {
	result, err := r.db.Exec(
		`INSERT INTO chart_definitions (name, trace_id, chart_type, is_built_in, sources)
		 VALUES (?, ?, ?, ?, ?)`,
		cd.Name, cd.TraceID, cd.ChartType, cd.IsBuiltIn, cd.Sources,
	)
	if err != nil {
		return fmt.Errorf("create chart definition: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	cd.ID = id
	return nil
}

// Get retrieves a chart definition by ID.
func (r *ChartRepo) Get(id int64) (*model.ChartDefinition, error) {
	cd := &model.ChartDefinition{}
	var traceID sql.NullInt64
	err := r.db.QueryRow(
		`SELECT id, name, trace_id, chart_type, is_built_in, sources, created_at
		 FROM chart_definitions WHERE id = ?`, id,
	).Scan(&cd.ID, &cd.Name, &traceID, &cd.ChartType, &cd.IsBuiltIn, &cd.Sources, &cd.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get chart definition: %w", err)
	}
	if traceID.Valid {
		cd.TraceID = &traceID.Int64
	}
	return cd, nil
}

// List returns all chart definitions that apply to a given trace (global + trace-specific).
func (r *ChartRepo) List(traceID *int64) ([]*model.ChartDefinition, error) {
	var rows *sql.Rows
	var err error
	if traceID != nil {
		rows, err = r.db.Query(
			`SELECT id, name, trace_id, chart_type, is_built_in, sources, created_at
			 FROM chart_definitions
			 WHERE trace_id IS NULL OR trace_id = ?
			 ORDER BY is_built_in DESC, id ASC`, *traceID,
		)
	} else {
		rows, err = r.db.Query(
			`SELECT id, name, trace_id, chart_type, is_built_in, sources, created_at
			 FROM chart_definitions
			 WHERE trace_id IS NULL
			 ORDER BY is_built_in DESC, id ASC`,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list chart definitions: %w", err)
	}
	defer rows.Close()

	var result []*model.ChartDefinition
	for rows.Next() {
		cd := &model.ChartDefinition{}
		var tid sql.NullInt64
		if err := rows.Scan(&cd.ID, &cd.Name, &tid, &cd.ChartType, &cd.IsBuiltIn, &cd.Sources, &cd.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chart definition: %w", err)
		}
		if tid.Valid {
			cd.TraceID = &tid.Int64
		}
		result = append(result, cd)
	}
	return result, rows.Err()
}

// Update modifies an existing chart definition.
func (r *ChartRepo) Update(cd *model.ChartDefinition) error {
	_, err := r.db.Exec(
		`UPDATE chart_definitions SET name = ?, chart_type = ?, sources = ? WHERE id = ?`,
		cd.Name, cd.ChartType, cd.Sources, cd.ID,
	)
	if err != nil {
		return fmt.Errorf("update chart definition: %w", err)
	}
	return nil
}

// Delete removes a chart definition. Returns an error if the chart is built-in.
func (r *ChartRepo) Delete(id int64) error {
	cd, err := r.Get(id)
	if err != nil {
		return err
	}
	if cd == nil {
		return fmt.Errorf("chart definition not found")
	}
	if cd.IsBuiltIn {
		return fmt.Errorf("cannot delete built-in chart")
	}
	_, err = r.db.Exec("DELETE FROM chart_definitions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete chart definition: %w", err)
	}
	return nil
}
