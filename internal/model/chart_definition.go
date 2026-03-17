package model

import "time"

// ChartDefinition represents a saved chart configuration.
type ChartDefinition struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	TraceID   *int64    `json:"traceId,omitempty"` // nil = global (applies to all traces)
	ChartType string    `json:"chartType"`         // "line" or "step"
	IsBuiltIn bool      `json:"isBuiltIn"`
	Sources   string    `json:"sources"` // JSON-encoded []ChartSource
	CreatedAt time.Time `json:"createdAt"`
}

// ChartSource describes one data source within a chart definition.
type ChartSource struct {
	FunctionSet  string   `json:"functionSet"`
	CmdKey       string   `json:"cmdKey"`
	DataArrayKey string   `json:"dataArrayKey"`
	IDField      string   `json:"idField"`
	Classifiers  []string `json:"classifiers"`
	FilterIDs    []string `json:"filterIds,omitempty"` // empty = all IDs
}
