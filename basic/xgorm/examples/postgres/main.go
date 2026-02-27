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

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/codesjoy/pkg/basic/xgorm"
	postgresdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const defaultExampleDSN = "postgres://xgorm:xgorm@127.0.0.1:5432/xgorm_example?sslmode=disable"

type exampleUser struct {
	ID      uint `gorm:"primaryKey"`
	Name    string
	Age     int
	Balance int
}

func (exampleUser) TableName() string {
	return "xgorm_example_users"
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dsn := strings.TrimSpace(os.Getenv("XGORM_DSN"))
	if dsn == "" {
		dsn = defaultExampleDSN
	}

	db, err := xgorm.New(postgresdriver.Open(dsn))
	if err != nil {
		fail(fmt.Errorf("create xgorm db: %w", err))
	}
	defer closeDB(db)

	db = db.WithContext(ctx)

	if err := db.AutoMigrate(&exampleUser{}); err != nil {
		fail(fmt.Errorf("auto migrate: %w", err))
	}
	if err := db.Where("1 = 1").Delete(&exampleUser{}).Error; err != nil {
		fail(fmt.Errorf("cleanup table: %w", err))
	}

	seed := []exampleUser{
		{Name: "alice", Age: 20, Balance: 100},
		{Name: "bob", Age: 24, Balance: 150},
		{Name: "carol", Age: 31, Balance: 220},
	}
	if err := db.Create(&seed).Error; err != nil {
		fail(fmt.Errorf("seed users: %w", err))
	}

	var page []exampleUser
	pageResult, err := xgorm.WrapPageQuery(
		db.Model(&exampleUser{}).Order("id ASC"),
		xgorm.PaginationParam{Pagination: true, Current: 1, PageSize: 2},
		&page,
	)
	if err != nil {
		fail(fmt.Errorf("wrap page query: %w", err))
	}
	fmt.Printf(
		"pagination total=%d current=%d size=%d rows=%d\n",
		pageResult.Total,
		pageResult.Current,
		pageResult.PageSize,
		len(page),
	)

	trans := xgorm.NewTransaction(db)
	err = trans.Transaction(ctx, func(tx *gorm.DB) error {
		return tx.Create(&exampleUser{Name: "tx-user", Age: 40, Balance: 999}).Error
	})
	if err != nil {
		fail(fmt.Errorf("transaction helper: %w", err))
	}
	fmt.Println("transaction helper committed one row")

	var foundUser exampleUser
	found, err := xgorm.FindOne(db.Model(&exampleUser{}).Where("name = ?", "alice"), &foundUser)
	if err != nil {
		fail(fmt.Errorf("find one: %w", err))
	}
	fmt.Printf(
		"find one for alice: found=%t id=%d balance=%d\n",
		found,
		foundUser.ID,
		foundUser.Balance,
	)

	var allRows []exampleUser
	countResult, err := xgorm.WrapPageQuery(
		db.Model(&exampleUser{}),
		xgorm.PaginationParam{OnlyCount: true},
		&allRows,
	)
	if err != nil {
		fail(fmt.Errorf("count only query: %w", err))
	}
	fmt.Printf("count only total=%d\n", countResult.Total)
}

func closeDB(db *gorm.DB) {
	if db == nil {
		return
	}
	_ = xgorm.CloseMetrics(db)
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	_ = sqlDB.Close()
}

func fail(err error) {
	if err == nil {
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
