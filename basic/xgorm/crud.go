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

package xgorm

import (
	"errors"

	"gorm.io/gorm"

	internalpkg "github.com/codesjoy/pkg/basic/xgorm/internal"
)

// PaginationParam defines parameters for paginated queries.
type PaginationParam struct {
	Pagination bool   // Whether to enable pagination.
	OnlyCount  bool   // Whether to only return the total count.
	NoCount    bool   // Whether to skip the count query.
	Current    uint32 // Current page number (1-based).
	PageSize   uint32 // Number of rows per page.
}

// GetCurrent returns the current page number.
func (a PaginationParam) GetCurrent() uint32 {
	return a.Current
}

// GetPageSize returns the page size, defaulting to 100 if unset.
func (a PaginationParam) GetPageSize() uint32 {
	pageSize := a.PageSize
	if a.PageSize == 0 {
		pageSize = 100
	}
	return pageSize
}

// PaginationResult contains pagination metadata returned by WrapPageQuery.
type PaginationResult struct {
	Total    uint32 // Total number of matching rows.
	Current  uint32 // Current page number.
	PageSize uint32 // Number of rows per page.
}

// WrapPageQuery executes a query with optional pagination.
// Depending on PaginationParam settings, it either returns all results,
// only the count, or a paginated subset with pagination metadata.
func WrapPageQuery(db *gorm.DB, pp PaginationParam, out interface{}) (*PaginationResult, error) {
	// Count-only mode: return just the total without fetching rows.
	if pp.OnlyCount {
		count, err := countRows(db, out)
		if err != nil {
			return nil, err
		}
		return &PaginationResult{Total: uint32(count)}, nil
	} else if !pp.Pagination {
		// Non-paginated mode: fetch all matching rows.
		err := db.Find(out).Error
		if err != nil {
			return nil, NewPaginationError("find", err)
		}
		return nil, nil
	}

	// Paginated mode: count then fetch the requested page.
	total, err := findPage(db, pp, out)
	if err != nil {
		return nil, err
	}

	return &PaginationResult{
		Total:    uint32(total),
		Current:  pp.GetCurrent(),
		PageSize: pp.GetPageSize(),
	}, nil
}

// findPage executes a paginated query: counts total rows, then fetches the requested page.
func findPage(db *gorm.DB, pp PaginationParam, out interface{}) (int64, error) {
	// Count total rows unless NoCount is set.
	var count int64
	if !pp.NoCount {
		var err error
		count, err = countRows(db, out)
		if err != nil {
			return 0, err
		} else if count == 0 {
			// Short-circuit: no rows match the filter.
			return count, nil
		}
	}

	// Apply offset and limit based on pagination parameters.
	current, pageSize := pp.GetCurrent(), pp.GetPageSize()
	if current > 0 && pageSize > 0 {
		db = db.Offset((int(current) - 1) * int(pageSize)).Limit(int(pageSize))
	} else if pageSize > 0 {
		db = db.Limit(int(pageSize))
	}

	// Execute the query and return results.
	err := db.Find(out).Error
	if err != nil {
		return count, NewPaginationError("find", err)
	}
	return count, nil
}

// FindOne fetches a single row. It returns (false, nil) when no record is found.
func FindOne(db *gorm.DB, out interface{}) (bool, error) {
	result := db.First(out)
	if err := result.Error; err != nil {
		// Record not found is not treated as an error.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// rowSliceElement extracts the element type from a slice/map pointer via reflection.
// It wraps the internal implementation and maps errors to the package-level types.
func rowSliceElement(rowsSlicePtr interface{}) (interface{}, error) {
	out, err := internalpkg.RowSliceElement(rowsSlicePtr)
	if err != nil {
		if errors.Is(err, internalpkg.ErrInvalidSliceType) {
			return nil, ErrInvalidSliceType
		}
		return nil, err
	}
	return out, nil
}

// countRows determines the total number of rows for the given query by extracting
// the model type from the output slice and executing a COUNT query.
func countRows(db *gorm.DB, out interface{}) (int64, error) {
	// Derive the model type from the output slice pointer.
	table, err := rowSliceElement(out)
	if err != nil {
		return 0, NewPaginationError("model", err)
	}

	// Execute the count query using the derived model.
	var count int64
	if err := db.Model(table).Count(&count).Error; err != nil {
		return 0, NewPaginationError("count", err)
	}
	return count, nil
}
