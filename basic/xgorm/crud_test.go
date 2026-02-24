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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Test model
type User struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:255"`
	Age  int    `gorm:"index"`
}

func TestWrapPageQuery_OnlyCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Insert test data
	users := []User{
		{Name: "Alice", Age: 30},
		{Name: "Bob", Age: 25},
		{Name: "Charlie", Age: 35},
	}
	for _, u := range users {
		err = db.Create(&u).Error
		require.NoError(t, err)
	}

	// Test OnlyCount
	var result []User
	pp := PaginationParam{
		OnlyCount: true,
	}
	res, err := WrapPageQuery(db, pp, &result)
	require.NoError(t, err)
	assert.Equal(t, uint32(3), res.Total)
	assert.Equal(t, uint32(0), res.Current)
	assert.Equal(t, uint32(0), res.PageSize)
	assert.Empty(t, result, "Result slice should be empty when OnlyCount is true")
}

func TestWrapPageQuery_NoPagination(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Insert test data
	users := []User{
		{Name: "Alice", Age: 30},
		{Name: "Bob", Age: 25},
	}
	for _, u := range users {
		err = db.Create(&u).Error
		require.NoError(t, err)
	}

	// Test without pagination
	var result []User
	pp := PaginationParam{
		Pagination: false,
	}
	res, err := WrapPageQuery(db, pp, &result)
	require.NoError(t, err)
	assert.Nil(t, res, "Result should be nil when pagination is disabled")
	assert.Len(t, result, 2, "Should return all users")
}

func TestWrapPageQuery_WithPagination(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Insert test data
	for i := 1; i <= 25; i++ {
		user := User{Name: "User", Age: i}
		err = db.Create(&user).Error
		require.NoError(t, err)
	}

	// Test pagination: page 2, page size 10
	var result []User
	pp := PaginationParam{
		Pagination: true,
		Current:    2,
		PageSize:   10,
	}
	res, err := WrapPageQuery(db, pp, &result)
	require.NoError(t, err)
	assert.Equal(t, uint32(25), res.Total)
	assert.Equal(t, uint32(2), res.Current)
	assert.Equal(t, uint32(10), res.PageSize)
	assert.Len(t, result, 10, "Should return 10 users")
}

func TestWrapPageQuery_DefaultPageSize(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Insert test data
	for i := 1; i <= 150; i++ {
		user := User{Name: "User", Age: i}
		err = db.Create(&user).Error
		require.NoError(t, err)
	}

	// Test with PageSize = 0 (should use default 100)
	var result []User
	pp := PaginationParam{
		Pagination: true,
		Current:    1,
		PageSize:   0,
	}
	res, err := WrapPageQuery(db, pp, &result)
	require.NoError(t, err)
	assert.Equal(t, uint32(100), res.PageSize)
	assert.Len(t, result, 100, "Should return 100 users (default page size)")
}

func TestWrapPageQuery_NoCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Insert test data
	for i := 1; i <= 10; i++ {
		user := User{Name: "User", Age: i}
		err = db.Create(&user).Error
		require.NoError(t, err)
	}

	// Test with NoCount = true
	var result []User
	pp := PaginationParam{
		Pagination: true,
		NoCount:    true,
		Current:    1,
		PageSize:   5,
	}
	res, err := WrapPageQuery(db, pp, &result)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), res.Total, "Total should be 0 when NoCount is true")
	assert.Len(t, result, 5, "Should still return 5 users")
}

func TestWrapPageQuery_EmptyResult(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Test pagination with no data
	var result []User
	pp := PaginationParam{
		Pagination: true,
		Current:    1,
		PageSize:   10,
	}
	res, err := WrapPageQuery(db, pp, &result)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), res.Total)
	assert.Empty(t, result)
}

func TestWrapPageQuery_Offset(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Insert test data with known ages
	for i := 1; i <= 10; i++ {
		user := User{Name: "User", Age: i}
		err = db.Create(&user).Error
		require.NoError(t, err)
	}

	// Test offset with PageSize but Current = 0
	var result []User
	pp := PaginationParam{
		Pagination: true,
		Current:    0,
		PageSize:   5,
	}
	res, err := WrapPageQuery(db, pp, &result)
	require.NoError(t, err)
	assert.Len(t, result, 5, "Should return first 5 users")
	assert.Equal(t, uint32(10), res.Total)
}

func TestFindOne_Exists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Insert test user
	user := User{Name: "Alice", Age: 30}
	err = db.Create(&user).Error
	require.NoError(t, err)

	// Find the user
	var found User
	exists, err := FindOne(db.Where("name = ?", "Alice"), &found)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "Alice", found.Name)
	assert.Equal(t, 30, found.Age)
}

func TestFindOne_NotExists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Try to find non-existent user
	var found User
	exists, err := FindOne(db.Where("name = ?", "NonExistent"), &found)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestFindOne_WithError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Invalid query should return error
	var found User
	_, err = FindOne(db.Where("invalid_column = ?", "test"), &found)
	assert.Error(t, err)
}

func TestPaginationParam_GetCurrent(t *testing.T) {
	pp := PaginationParam{Current: 5}
	assert.Equal(t, uint32(5), pp.GetCurrent())
}

func TestPaginationParam_GetPageSize_Default(t *testing.T) {
	pp := PaginationParam{PageSize: 0}
	assert.Equal(t, uint32(100), pp.GetPageSize(), "Default page size should be 100")
}

func TestPaginationParam_GetPageSize_Custom(t *testing.T) {
	pp := PaginationParam{PageSize: 25}
	assert.Equal(t, uint32(25), pp.GetPageSize())
}

func TestWrapPageQuery_InvalidSlice(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Pass non-slice value (should error from rowSliceElement)
	pp := PaginationParam{
		OnlyCount: true,
	}
	var result User // Not a slice
	_, err = WrapPageQuery(db, pp, &result)
	assert.Error(t, err)
	assert.True(t, IsPaginationError(err))
	assert.True(t, IsInvalidSliceType(err))

	var pgErr *PaginationError
	require.True(t, errors.As(err, &pgErr))
	assert.Equal(t, "model", pgErr.Operation)
}

func TestWrapPageQuery_WithQuery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Insert test data
	for i := 1; i <= 10; i++ {
		user := User{Name: "User", Age: i}
		err = db.Create(&user).Error
		require.NoError(t, err)
	}

	// Test pagination with query condition
	var result []User
	pp := PaginationParam{
		Pagination: true,
		Current:    1,
		PageSize:   5,
	}

	// Only get users with Age >= 6
	query := db.Where("age >= ?", 6)
	res, err := WrapPageQuery(query, pp, &result)
	require.NoError(t, err)
	assert.Equal(t, uint32(5), res.Total, "Should find 5 users with age >= 6")
	assert.Len(t, result, 5)

	// Verify ages
	for _, u := range result {
		assert.GreaterOrEqual(t, u.Age, 6)
	}
}

func TestWrapPageQuery_ErrorHandling(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{})
	require.NoError(t, err)

	// Close the database to induce errors
	sqlDB, _ := db.DB()
	sqlDB.Close()

	var result []User
	pp := PaginationParam{
		OnlyCount: true,
	}
	_, err = WrapPageQuery(db, pp, &result)
	assert.Error(t, err, "Should return error when database is closed")
	assert.True(t, IsPaginationError(err))

	var pgErr *PaginationError
	require.True(t, errors.As(err, &pgErr))
	assert.Equal(t, "count", pgErr.Operation)
}

// Benchmark tests
func BenchmarkWrapPageQuery_WithCount(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = db.AutoMigrate(&User{})
	require.NoError(b, err)

	// Insert test data
	for i := 0; i < 1000; i++ {
		user := User{Name: "User", Age: i}
		err = db.Create(&user).Error
		require.NoError(b, err)
	}

	pp := PaginationParam{
		Pagination: true,
		Current:    1,
		PageSize:   50,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result []User
		_, _ = WrapPageQuery(db, pp, &result)
	}
}

func BenchmarkWrapPageQuery_NoCount(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = db.AutoMigrate(&User{})
	require.NoError(b, err)

	// Insert test data
	for i := 0; i < 1000; i++ {
		user := User{Name: "User", Age: i}
		err = db.Create(&user).Error
		require.NoError(b, err)
	}

	pp := PaginationParam{
		Pagination: true,
		NoCount:    true,
		Current:    1,
		PageSize:   50,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var result []User
		_, _ = WrapPageQuery(db, pp, &result)
	}
}

func BenchmarkFindOne(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = db.AutoMigrate(&User{})
	require.NoError(b, err)

	user := User{Name: "Alice", Age: 30}
	err = db.Create(&user).Error
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var found User
		_, _ = FindOne(db.Where("name = ?", "Alice"), &found)
	}
}
