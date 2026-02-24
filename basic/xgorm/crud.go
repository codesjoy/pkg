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

// PaginationParam 分页查询条件
type PaginationParam struct {
	Pagination bool   // 是否使用分页查询
	OnlyCount  bool   // 是否仅查询count
	NoCount    bool   // 不需要进行count
	Current    uint32 // 当前页
	PageSize   uint32 // 页大小
}

// GetCurrent 获取当前页
func (a PaginationParam) GetCurrent() uint32 {
	return a.Current
}

// GetPageSize 获取页大小
func (a PaginationParam) GetPageSize() uint32 {
	pageSize := a.PageSize
	if a.PageSize == 0 {
		pageSize = 100
	}
	return pageSize
}

// PaginationResult contains pagination metadata returned by WrapPageQuery.
type PaginationResult struct {
	Total    uint32
	Current  uint32
	PageSize uint32
}

// WrapPageQuery 包装带有分页的查询
func WrapPageQuery(db *gorm.DB, pp PaginationParam, out interface{}) (*PaginationResult, error) {
	if pp.OnlyCount {
		count, err := countRows(db, out)
		if err != nil {
			return nil, err
		}
		return &PaginationResult{Total: uint32(count)}, nil
	} else if !pp.Pagination {
		err := db.Find(out).Error
		if err != nil {
			return nil, NewPaginationError("find", err)
		}
		return nil, nil
	}

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

// FindPage 查询分页数据
func findPage(db *gorm.DB, pp PaginationParam, out interface{}) (int64, error) {
	var count int64
	if !pp.NoCount {
		var err error
		count, err = countRows(db, out)
		if err != nil {
			return 0, err
		} else if count == 0 {
			return count, nil
		}
	}

	current, pageSize := pp.GetCurrent(), pp.GetPageSize()
	if current > 0 && pageSize > 0 {
		db = db.Offset((int(current) - 1) * int(pageSize)).Limit(int(pageSize))
	} else if pageSize > 0 {
		db = db.Limit(int(pageSize))
	}

	err := db.Find(out).Error
	if err != nil {
		return count, NewPaginationError("find", err)
	}
	return count, nil
}

// FindOne 查询单条数据
func FindOne(db *gorm.DB, out interface{}) (bool, error) {
	result := db.First(out)
	if err := result.Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func rowSliceElement(rowsSlicePtr interface{}) (interface{}, error) {
	// Use the internal implementation
	out, err := internalpkg.RowSliceElement(rowsSlicePtr)
	if err != nil {
		if errors.Is(err, internalpkg.ErrInvalidSliceType) {
			return nil, ErrInvalidSliceType
		}
		return nil, err
	}
	return out, nil
}

func countRows(db *gorm.DB, out interface{}) (int64, error) {
	table, err := rowSliceElement(out)
	if err != nil {
		return 0, NewPaginationError("model", err)
	}

	var count int64
	if err := db.Model(table).Count(&count).Error; err != nil {
		return 0, NewPaginationError("count", err)
	}
	return count, nil
}
