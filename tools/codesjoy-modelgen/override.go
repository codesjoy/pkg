package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// OverrideConfig controls table/column-level generation overrides.
type OverrideConfig struct {
	IncludeTables []string                 `yaml:"include_tables"`
	ExcludeTables []string                 `yaml:"exclude_tables"`
	GenAIPSQL     *bool                    `yaml:"gen_aipsql"`
	TimestampMode string                   `yaml:"timestamp_mode"`
	Tables        map[string]TableOverride `yaml:"tables"`
}

// TableOverride controls one table.
type TableOverride struct {
	Skip          *bool                     `yaml:"skip"`
	ModelName     string                    `yaml:"model_name"`
	AIPSQLBuilder string                    `yaml:"aipsql_builder"`
	GenAIPSQL     *bool                     `yaml:"gen_aipsql"`
	TimestampMode string                    `yaml:"timestamp_mode"`
	Columns       map[string]ColumnOverride `yaml:"columns"`
}

// ColumnOverride controls one column.
type ColumnOverride struct {
	Skip           *bool    `yaml:"skip"`
	GoField        string   `yaml:"go_field"`
	GoType         string   `yaml:"go_type"`
	JSONName       string   `yaml:"json_name"`
	FieldPath      string   `yaml:"field_path"`
	Filterable     *bool    `yaml:"filterable"`
	Sortable       *bool    `yaml:"sortable"`
	ImplicitFilter *bool    `yaml:"implicit_filter"`
	BoolType       *bool    `yaml:"bool_type"`
	KeyValue       *bool    `yaml:"key_value"`
	MatchModes     []string `yaml:"match_modes"`
	IndexHint      string   `yaml:"index_hint"`
	GormTagAppend  string   `yaml:"gorm_tag_append"`
	TimestampMode  string   `yaml:"timestamp_mode"`
}

// LoadOverrideConfig loads YAML overrides from disk.
func LoadOverrideConfig(path string) (OverrideConfig, error) {
	if strings.TrimSpace(path) == "" {
		return OverrideConfig{}, nil
	}
	// #nosec G304 -- path is a user-provided config location by design.
	content, err := os.ReadFile(path)
	if err != nil {
		return OverrideConfig{}, fmt.Errorf("read override file %s: %w", path, err)
	}
	var cfg OverrideConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return OverrideConfig{}, fmt.Errorf("parse override yaml %s: %w", path, err)
	}
	if cfg.Tables == nil {
		cfg.Tables = map[string]TableOverride{}
	}
	if err := validateOverrideTimestampModes(cfg); err != nil {
		return OverrideConfig{}, err
	}
	cfg.IncludeTables = dedupeStrings(cfg.IncludeTables)
	cfg.ExcludeTables = dedupeStrings(cfg.ExcludeTables)
	return cfg, nil
}

func (c OverrideConfig) tableOverride(tableName string) (TableOverride, bool) {
	if len(c.Tables) == 0 {
		return TableOverride{}, false
	}
	if ov, ok := c.Tables[tableName]; ok {
		return ov, true
	}
	if ov, ok := c.Tables[strings.ToLower(tableName)]; ok {
		return ov, true
	}
	return TableOverride{}, false
}

func tableColumnOverride(tableOv TableOverride, columnName string) (ColumnOverride, bool) {
	if len(tableOv.Columns) == 0 {
		return ColumnOverride{}, false
	}
	if ov, ok := tableOv.Columns[columnName]; ok {
		return ov, true
	}
	if ov, ok := tableOv.Columns[strings.ToLower(columnName)]; ok {
		return ov, true
	}
	return ColumnOverride{}, false
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func validateOverrideTimestampModes(cfg OverrideConfig) error {
	if strings.TrimSpace(cfg.TimestampMode) != "" {
		if err := validateTimestampMode(cfg.TimestampMode); err != nil {
			return err
		}
	}
	for tableName, table := range cfg.Tables {
		if strings.TrimSpace(table.TimestampMode) != "" {
			if err := validateTimestampMode(table.TimestampMode); err != nil {
				return fmt.Errorf("table %s: %w", tableName, err)
			}
		}
		for columnName, col := range table.Columns {
			if strings.TrimSpace(col.TimestampMode) == "" {
				continue
			}
			if err := validateTimestampMode(col.TimestampMode); err != nil {
				return fmt.Errorf("table %s column %s: %w", tableName, columnName, err)
			}
		}
	}
	return nil
}
