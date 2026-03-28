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
	"strings"
	"testing"

	libraryv1 "github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1"
)

func TestTypedFillHelpers(t *testing.T) {
	publisher := &libraryv1.Publisher{}
	if err := publisher.FillNameFromParts(libraryv1.PublisherNameParts{Publisher: "pub-1"}); err != nil {
		t.Fatalf("FillNameFromParts() error = %v", err)
	}
	if got, want := publisher.GetName(), "publishers/pub-1"; got != want {
		t.Fatalf("publisher name = %q, want %q", got, want)
	}

	book := &libraryv1.Book{}
	err := book.FillNameWithPatternFromParts(
		libraryv1.BookNamePattern1,
		libraryv1.BookNameParts{
			Publisher: "pub-1",
			Book:      "book-1",
			Archive:   "ignored-archive",
		},
	)
	if err != nil {
		t.Fatalf("FillNameWithPatternFromParts() error = %v", err)
	}
	if got, want := book.GetName(), "publishers/pub-1/books/book-1"; got != want {
		t.Fatalf("book name = %q, want %q", got, want)
	}
}

func TestTypedFormatValidation(t *testing.T) {
	_, err := libraryv1.FormatPublisherName(libraryv1.PublisherNameParts{})
	if err == nil || !strings.Contains(err.Error(), `must not be empty`) {
		t.Fatalf("FormatPublisherName() error = %v, want empty-value error", err)
	}

	_, err = libraryv1.FormatBookNameWithPattern(
		libraryv1.BookNamePattern1,
		libraryv1.BookNameParts{
			Publisher: "pub/1",
			Book:      "book-1",
		},
	)
	if err == nil || !strings.Contains(err.Error(), `must not contain '/'`) {
		t.Fatalf("FormatBookNameWithPattern() error = %v, want slash-validation error", err)
	}
}

func TestDeprecatedMapFillRetainsMissingKeyBehavior(t *testing.T) {
	book := &libraryv1.Book{}
	err := book.FillNameWithPattern(libraryv1.BookNamePattern1, map[string]string{
		"publisher": "pub-1",
	})
	if err == nil || !strings.Contains(err.Error(), `missing value for variable "book"`) {
		t.Fatalf("FillNameWithPattern() error = %v, want missing-key error", err)
	}
}

func TestInvalidPatternStillErrors(t *testing.T) {
	_, err := libraryv1.FormatBookNameWithPattern(
		"books/{book}",
		libraryv1.BookNameParts{Book: "book-1"},
	)
	if err == nil || !strings.Contains(err.Error(), `pattern "books/{book}" is not registered`) {
		t.Fatalf("FormatBookNameWithPattern() error = %v, want invalid-pattern error", err)
	}
}
