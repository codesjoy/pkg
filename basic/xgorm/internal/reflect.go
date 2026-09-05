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

// Package internal contains internal utilities for the xgorm package.
package internal

import (
	"errors"
	"reflect"
)

// ErrInvalidSliceType is returned when the provided slice has an invalid type.
var ErrInvalidSliceType = errors.New("needs a pointer to a slice or a map")

// RowSliceElement extracts the element type from a slice or map pointer.
// It takes a pointer to a slice or map and returns a pointer to a new instance
// of the element type. This is commonly used in GORM to get the model type from
// a slice pointer for .Model() calls.
//
// For example:
//
//	users := &[]User{}
//	model, err := RowSliceElement(users)  // Returns &User{}
//	db.Model(model).Count(&count)
//
// The function handles:
// - Pointer to slices: []*T → *T
// - Pointer to slices of value types: []T → *T
// - Pointer to maps: map[K]V → *V
//
// Returns an error if the input is not a pointer to a slice or map.
func RowSliceElement(rowsSlicePtr interface{}) (interface{}, error) {
	if rowsSlicePtr == nil {
		return nil, ErrInvalidSliceType
	}

	t := reflect.TypeOf(rowsSlicePtr)
	if t.Kind() != reflect.Pointer {
		return nil, ErrInvalidSliceType
	}
	if reflect.ValueOf(rowsSlicePtr).IsNil() {
		return nil, ErrInvalidSliceType
	}

	containerType := t.Elem()
	// Check if we have a slice or map
	if containerType.Kind() != reflect.Slice && containerType.Kind() != reflect.Map {
		return nil, ErrInvalidSliceType
	}

	// Get the element type
	sliceElementType := containerType.Elem()

	// If it's a pointer type (e.g., []*User), get the underlying type
	if sliceElementType.Kind() == reflect.Pointer {
		sliceElementType = sliceElementType.Elem()
	}

	// Return a pointer to a new instance of the element type
	return reflect.New(sliceElementType).Interface(), nil
}

// IsNil safely checks if an interface{} value is nil.
// This is safer than checking "== nil" directly because it handles
// typed nil pointers correctly.
func IsNil(v interface{}) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.Interface:
		return rv.IsNil()
	}

	return false
}

// GetTypeName returns the type name of a value.
// For pointers, it returns the type name of the underlying type.
func GetTypeName(v interface{}) string {
	if v == nil {
		return "nil"
	}

	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.String()
}

// IsSlice checks if a value is a slice or pointer to a slice.
func IsSlice(v interface{}) bool {
	if v == nil {
		return false
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	return rv.Kind() == reflect.Slice
}

// IsMap checks if a value is a map or pointer to a map.
func IsMap(v interface{}) bool {
	if v == nil {
		return false
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	return rv.Kind() == reflect.Map
}

// SliceLen returns the length of a slice.
// Returns 0 if the value is not a slice or is nil.
func SliceLen(v interface{}) int {
	if v == nil {
		return 0
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}

	if rv.Kind() == reflect.Slice {
		return rv.Len()
	}

	return 0
}
