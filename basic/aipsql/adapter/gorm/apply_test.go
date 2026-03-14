// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aipsqlgorm

import (
	"testing"
	"time"

	aip "github.com/codesjoy/pkg/basic/aipsql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type orderRecord struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	Status    string    `gorm:"column:status"`
	Name      string    `gorm:"column:name"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (orderRecord) TableName() string {
	return "orders"
}

func TestApplyWhere(t *testing.T) {
	db := setupOrderDB(t)

	table := aip.NewTable().WithColumns(
		aip.NewColumn().
			WithFieldPath("status").
			WithDatabaseName("status").
			WithMatchModes(aip.MatchModeExact).
			Filterable().
			Build(),
		aip.NewColumn().
			WithFieldPath("name").
			WithDatabaseName("name").
			WithMatchModes(aip.MatchModePrefix, aip.MatchModeContains).
			Filterable().
			Build(),
	).Build()

	filter, err := aip.ParseFilter(`status="active" AND name="Alice"`)
	require.NoError(t, err)

	whereSQL, params, err := table.WhereClause(filter, "p_")
	require.NoError(t, err)

	var rows []orderRecord
	err = ApplyWhere(db.Model(&orderRecord{}), whereSQL, params).
		Order("id ASC").
		Find(&rows).Error
	require.NoError(t, err)
	assertOrderIDs(t, rows, 1)
	assert.Equal(t, "Alice", rows[0].Name)
}

func TestApplyPlan(t *testing.T) {
	db := setupOrderDB(t)

	plan := &aip.QueryPlan{
		WhereClause:   "(status = @p_0)",
		OrderByClause: "created_at DESC, id DESC",
		Limit:         2,
		Offset:        1,
		Parameters: []aip.QueryParameter{
			{Name: "p_0", Value: "active"},
		},
	}

	var rows []orderRecord
	err := ApplyPlan(db.Model(&orderRecord{}), plan).Find(&rows).Error
	require.NoError(t, err)
	assertOrderIDs(t, rows, 3, 1)
}

func TestApplyWhereEmptyClauseNoop(t *testing.T) {
	db := setupOrderDB(t).Model(&orderRecord{})
	assert.Same(t, db, ApplyWhere(db, "", nil))
}

func TestApplyPlanNilNoop(t *testing.T) {
	db := setupOrderDB(t).Model(&orderRecord{})
	assert.Same(t, db, ApplyPlan(db, nil))
}

func setupOrderDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&orderRecord{})
	require.NoError(t, err)

	seed := []orderRecord{
		{
			ID:        1,
			Status:    "active",
			Name:      "Alice",
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        2,
			Status:    "pending",
			Name:      "Bob",
			CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        3,
			Status:    "active",
			Name:      "Alex",
			CreatedAt: time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        4,
			Status:    "active",
			Name:      "Alfred",
			CreatedAt: time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, row := range seed {
		require.NoError(t, db.Create(&row).Error)
	}

	return db
}

func assertOrderIDs(t *testing.T, rows []orderRecord, expected ...int64) {
	t.Helper()

	require.Len(t, rows, len(expected))
	for i, id := range expected {
		assert.Equal(t, id, rows[i].ID)
	}
}
