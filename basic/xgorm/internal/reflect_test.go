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

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test models
type User struct {
	ID   uint
	Name string
}

type Product struct {
	ID    uint
	Name  string
	Price float64
}

func TestRowSliceElement(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		wantErr     bool
		validateOut func(*testing.T, interface{})
	}{
		{
			name:    "pointer to slice of pointer structs",
			input:   &[]*User{},
			wantErr: false,
			validateOut: func(t *testing.T, out interface{}) {
				require.NotNil(t, out)
				user, ok := out.(*User)
				require.True(t, ok, "Should be *User type")
				assert.Equal(t, uint(0), user.ID)
				assert.Empty(t, user.Name)
			},
		},
		{
			name:    "pointer to slice of value structs",
			input:   &[]User{},
			wantErr: false,
			validateOut: func(t *testing.T, out interface{}) {
				require.NotNil(t, out)
				user, ok := out.(*User)
				require.True(t, ok, "Should be *User type")
				assert.Equal(t, uint(0), user.ID)
				assert.Empty(t, user.Name)
			},
		},
		{
			name:    "pointer to map",
			input:   &map[string]User{},
			wantErr: false,
			validateOut: func(t *testing.T, out interface{}) {
				require.NotNil(t, out)
				user, ok := out.(*User)
				require.True(t, ok, "Should be *User type")
				assert.Equal(t, uint(0), user.ID)
			},
		},
		{
			name:    "pointer to slice with elements",
			input:   &[]User{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}},
			wantErr: false,
			validateOut: func(t *testing.T, out interface{}) {
				require.NotNil(t, out)
				user, ok := out.(*User)
				require.True(t, ok, "Should be *User type")
				assert.Equal(t, uint(0), user.ID, "Should create new instance")
			},
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: true,
			validateOut: func(t *testing.T, out interface{}) {
				assert.Nil(t, out)
			},
		},
		{
			name:    "non-pointer slice",
			input:   []User{},
			wantErr: true,
			validateOut: func(t *testing.T, out interface{}) {
				assert.Nil(t, out)
			},
		},
		{
			name:    "pointer to struct (not slice/map)",
			input:   &User{ID: 1},
			wantErr: true,
			validateOut: func(t *testing.T, out interface{}) {
				assert.Nil(t, out)
			},
		},
		{
			name:    "pointer to int",
			input:   new(int),
			wantErr: true,
			validateOut: func(t *testing.T, out interface{}) {
				assert.Nil(t, out)
			},
		},
		{
			name:    "string value",
			input:   "test",
			wantErr: true,
			validateOut: func(t *testing.T, out interface{}) {
				assert.Nil(t, out)
			},
		},
		{
			name:    "pointer to map of pointers",
			input:   &map[int]*Product{},
			wantErr: false,
			validateOut: func(t *testing.T, out interface{}) {
				require.NotNil(t, out)
				product, ok := out.(*Product)
				require.True(t, ok, "Should be *Product type")
				assert.Equal(t, uint(0), product.ID)
			},
		},
		{
			name:    "pointer to slice of interfaces",
			input:   &[]interface{}{},
			wantErr: false,
			validateOut: func(t *testing.T, out interface{}) {
				require.NotNil(t, out)
				// Element type is interface{}, so we get *interface{}
				ifacePtr, ok := out.(*interface{})
				require.True(t, ok)
				assert.Nil(t, *ifacePtr)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := RowSliceElement(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, ErrInvalidSliceType, err)
			} else {
				assert.NoError(t, err)
				if tt.validateOut != nil {
					tt.validateOut(t, out)
				}
			}
		})
	}
}

func TestIsNil(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want bool
	}{
		{"nil interface", nil, true},
		{"nil pointer", (*User)(nil), true},
		{"nil slice", ([]User)(nil), true},
		{"nil map", (map[string]int)(nil), true},
		{"non-nil struct", User{ID: 1}, false},
		{"pointer to struct", &User{ID: 1}, false},
		{"empty slice", []User{}, false},
		{"empty map", map[string]int{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNil(tt.v)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetTypeName(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want string
	}{
		{"nil", nil, "nil"},
		{"struct", User{}, "internal.User"},
		{"pointer to struct", &User{}, "internal.User"},
		{"slice", []User{}, "[]internal.User"},
		{"pointer to slice", &[]User{}, "[]internal.User"},
		{"int", 42, "int"},
		{"pointer to int", new(int), "int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTypeName(tt.v)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsSlice(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want bool
	}{
		{"nil", nil, false},
		{"slice", []User{}, true},
		{"pointer to slice", &[]User{}, true},
		{"non-slice", User{}, false},
		{"pointer to struct", &User{}, false},
		{"map", map[string]int{}, false},
		{"int", 42, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSlice(tt.v)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsMap(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want bool
	}{
		{"nil", nil, false},
		{"map", map[string]int{}, true},
		{"pointer to map", &map[string]int{}, true},
		{"non-map", User{}, false},
		{"pointer to struct", &User{}, false},
		{"slice", []User{}, false},
		{"int", 42, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMap(tt.v)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSliceLen(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want int
	}{
		{"nil", nil, 0},
		{"nil slice", ([]User)(nil), 0},
		{"empty slice", []User{}, 0},
		{"slice with items", []User{{ID: 1}, {ID: 2}}, 2},
		{"pointer to empty slice", &[]User{}, 0},
		{"pointer to slice with items", &[]User{{ID: 1}, {ID: 2}}, 2},
		{"non-slice", User{}, 0},
		{"int", 42, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SliceLen(tt.v)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRowSliceElement_PointerToMap(t *testing.T) {
	// Test different map types
	tests := []struct {
		name     string
		input    interface{}
		wantType string
	}{
		{"map[string]*User", &map[string]*User{}, "User"},
		{"map[int]User", &map[int]User{}, "User"},
		{"map[string]interface{}", &map[string]interface{}{}, "interface {}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := RowSliceElement(tt.input)
			require.NoError(t, err)
			require.NotNil(t, out)

			typeName := GetTypeName(out)
			// Remove the * prefix if present for comparison
			if typeName[0] == '*' {
				typeName = typeName[1:]
			}
			// Handle interface{} special case
			if tt.wantType == "interface {}" {
				assert.Contains(t, typeName, "interface")
			} else {
				assert.Contains(t, typeName, tt.wantType)
			}
		})
	}
}

func TestRowSliceElement_DifferentSliceTypes(t *testing.T) {
	// Test various slice element types
	tests := []struct {
		name     string
		input    interface{}
		wantType string
	}{
		{"[]int", &[]int{}, "int"},
		{"[]*int", &[]*int{}, "int"},
		{"[]string", &[]string{}, "string"},
		{"[]*string", &[]*string{}, "string"},
		{"[]User", &[]User{}, "User"},
		{"[]*User", &[]*User{}, "User"},
		{"[]interface{}", &[]interface{}{}, "interface {}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := RowSliceElement(tt.input)
			require.NoError(t, err)
			require.NotNil(t, out)

			typeName := GetTypeName(out)
			// Remove the * prefix if present for comparison
			if typeName[0] == '*' {
				typeName = typeName[1:]
			}
			// Handle interface{} special case
			if tt.wantType == "interface {}" {
				assert.Contains(t, typeName, "interface")
			} else {
				assert.Contains(t, typeName, tt.wantType)
			}
		})
	}
}

// Benchmark RowSliceElement
func BenchmarkRowSliceElement(b *testing.B) {
	input := &[]User{{ID: 1}, {ID: 2}, {ID: 3}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = RowSliceElement(input)
	}
}

func BenchmarkRowSliceElement_Pointers(b *testing.B) {
	input := &[]*User{{ID: 1}, {ID: 2}, {ID: 3}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = RowSliceElement(input)
	}
}

func BenchmarkIsNil(b *testing.B) {
	v := &User{ID: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsNil(v)
	}
}

func BenchmarkGetTypeName(b *testing.B) {
	v := &User{ID: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetTypeName(v)
	}
}
