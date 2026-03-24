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

package xmongo

import (
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	// ErrEmptyURI indicates Config.URI is empty after trimming spaces.
	ErrEmptyURI = errors.New("mongodb uri is empty")
	// ErrEmptyDefaultDatabase indicates Config.DefaultDatabase contains only spaces.
	ErrEmptyDefaultDatabase = errors.New("mongodb default database is empty")
	// ErrDefaultDatabaseRequired indicates a default-database helper was used without configuration.
	ErrDefaultDatabaseRequired = errors.New("mongodb default database is required")
	// ErrEmptyCollectionName indicates Collection(...) was called with an empty name.
	ErrEmptyCollectionName = errors.New("mongodb collection name is empty")
	// ErrNilClientOption indicates one *options.ClientOptions is nil.
	ErrNilClientOption = errors.New("mongodb client option is nil")
	// ErrNilCommandMonitor indicates one command monitor is nil.
	ErrNilCommandMonitor = errors.New("mongodb command monitor is nil")
	// ErrNilPoolMonitor indicates one pool monitor is nil.
	ErrNilPoolMonitor = errors.New("mongodb pool monitor is nil")
	// ErrNilServerMonitor indicates one server monitor is nil.
	ErrNilServerMonitor = errors.New("mongodb server monitor is nil")
)

// Config contains client construction settings.
type Config struct {
	URI             string
	DefaultDatabase string
	ClientOptions   []*options.ClientOptions
}

// Validate validates and normalizes xmongo config.
func (c *Config) Validate() error {
	if c == nil {
		return ErrEmptyURI
	}

	c.URI = strings.TrimSpace(c.URI)
	if c.URI == "" {
		return ErrEmptyURI
	}

	defaultDatabaseProvided := c.DefaultDatabase != ""
	c.DefaultDatabase = strings.TrimSpace(c.DefaultDatabase)
	if defaultDatabaseProvided && c.DefaultDatabase == "" {
		return ErrEmptyDefaultDatabase
	}

	merged, err := mergeNativeClientOptions(c.URI, c.ClientOptions)
	if err != nil {
		return err
	}
	return merged.Validate()
}
