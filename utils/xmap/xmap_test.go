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

package xmap

import (
	"reflect"
	"testing"
)

func TestMergeStringMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dst  map[string]interface{}
		src  []map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "simple override and add",
			dst: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			src: []map[string]interface{}{
				{
					"key2": "new_value2",
					"key3": "value3",
				},
			},
			want: map[string]interface{}{
				"key1": "value1",
				"key2": "new_value2",
				"key3": "value3",
			},
		},
		{
			name: "nested merge with multiple sources",
			dst: map[string]interface{}{
				"server": map[string]interface{}{
					"host": "0.0.0.0",
					"port": 8080,
				},
				"database": map[string]interface{}{
					"host": "localhost",
					"port": 5432,
					"pool": map[string]interface{}{
						"max": 10,
						"min": 2,
					},
				},
			},
			src: []map[string]interface{}{
				{
					"server": map[string]interface{}{
						"port": 9000,
					},
					"database": map[string]interface{}{
						"host": "db.example.com",
						"pool": map[string]interface{}{
							"max": 20,
						},
					},
				},
				{
					"database": map[string]interface{}{
						"ssl": true,
					},
				},
			},
			want: map[string]interface{}{
				"server": map[string]interface{}{
					"host": "0.0.0.0",
					"port": 9000,
				},
				"database": map[string]interface{}{
					"host": "db.example.com",
					"port": 5432,
					"ssl":  true,
					"pool": map[string]interface{}{
						"max": 20,
						"min": 2,
					},
				},
			},
		},
		{
			name: "type mismatch is ignored",
			dst: map[string]interface{}{
				"key": "string_value",
			},
			src: []map[string]interface{}{
				{
					"key": 123,
				},
			},
			want: map[string]interface{}{
				"key": "string_value",
			},
		},
		{
			name: "map interface keys are normalized then merged",
			dst: map[string]interface{}{
				"cfg": map[interface{}]interface{}{
					"a": 1,
					"b": map[interface{}]interface{}{
						"x": "old",
					},
				},
			},
			src: []map[string]interface{}{
				{
					"cfg": map[interface{}]interface{}{
						"b": map[interface{}]interface{}{
							"y": "new",
						},
					},
				},
			},
			want: map[string]interface{}{
				"cfg": map[string]interface{}{
					"a": 1,
					"b": map[string]interface{}{
						"x": "old",
						"y": "new",
					},
				},
			},
		},
		{
			name: "empty sources keep destination unchanged",
			dst: map[string]interface{}{
				"key": "value",
			},
			src:  nil,
			want: map[string]interface{}{"key": "value"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			MergeStringMap(tc.dst, tc.src...)
			assertDeepEqual(t, tc.dst, tc.want)
		})
	}
}

func TestToMapStringInterface(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  map[interface{}]interface{}
		want map[string]interface{}
	}{
		{
			name: "mixed keys",
			src: map[interface{}]interface{}{
				"string": "value",
				123:      "int",
				true:     "bool",
				3.14:     "float",
			},
			want: map[string]interface{}{
				"string": "value",
				"123":    "int",
				"true":   "bool",
				"3.14":   "float",
			},
		},
		{
			name: "empty",
			src:  map[interface{}]interface{}{},
			want: map[string]interface{}{},
		},
		{
			name: "nested values are preserved",
			src: map[interface{}]interface{}{
				"nested": map[interface{}]interface{}{"inner": "value"},
			},
			want: map[string]interface{}{
				"nested": map[interface{}]interface{}{"inner": "value"},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ToMapStringInterface(tc.src)
			assertDeepEqual(t, got, tc.want)
		})
	}
}

func TestCoverInterfaceMapToStringMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "nested maps and slices are normalized",
			src: map[string]interface{}{
				"simple": "value",
				"nested": map[interface{}]interface{}{
					"key": "value",
				},
				"array": []interface{}{
					"string",
					123,
					map[interface{}]interface{}{
						"array_key": "array_value",
					},
				},
			},
			want: map[string]interface{}{
				"simple": "value",
				"nested": map[string]interface{}{
					"key": "value",
				},
				"array": []interface{}{
					"string",
					123,
					map[string]interface{}{
						"array_key": "array_value",
					},
				},
			},
		},
		{
			name: "complex structure",
			src: map[string]interface{}{
				"config": map[interface{}]interface{}{
					"servers": []interface{}{
						map[interface{}]interface{}{"host": "server1.example.com", "port": 8080},
						map[interface{}]interface{}{"host": "server2.example.com", "port": 8081},
					},
					"database": map[interface{}]interface{}{
						"primary": map[interface{}]interface{}{
							"host":     "db1.example.com",
							"port":     5432,
							"database": "production",
						},
					},
				},
			},
			want: map[string]interface{}{
				"config": map[string]interface{}{
					"servers": []interface{}{
						map[string]interface{}{"host": "server1.example.com", "port": 8080},
						map[string]interface{}{"host": "server2.example.com", "port": 8081},
					},
					"database": map[string]interface{}{
						"primary": map[string]interface{}{
							"host":     "db1.example.com",
							"port":     5432,
							"database": "production",
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			CoverInterfaceMapToStringMap(tc.src)
			assertDeepEqual(t, tc.src, tc.want)
		})
	}
}

func TestDeepSearchInMap(t *testing.T) {
	t.Parallel()

	t.Run("find nested value", func(t *testing.T) {
		t.Parallel()

		m := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"value": "found",
				},
			},
		}

		got := DeepSearchInMap(m, "level1", "level2", "value")
		if got != "found" {
			t.Fatalf("DeepSearchInMap() = %v, want %v", got, "found")
		}
	})

	t.Run("missing path returns nil", func(t *testing.T) {
		t.Parallel()

		m := map[string]interface{}{"key": "value"}
		got := DeepSearchInMap(m, "missing")
		if got != nil {
			t.Fatalf("DeepSearchInMap() = %v, want nil", got)
		}
	})

	t.Run("intermediate non map returns nil", func(t *testing.T) {
		t.Parallel()

		m := map[string]interface{}{"level1": "value"}
		got := DeepSearchInMap(m, "level1", "level2")
		if got != nil {
			t.Fatalf("DeepSearchInMap() = %v, want nil", got)
		}
	})

	t.Run("last path can be scalar", func(t *testing.T) {
		t.Parallel()

		m := map[string]interface{}{
			"config": map[string]interface{}{
				"port": 8080,
			},
		}
		got := DeepSearchInMap(m, "config", "port")
		if got != 8080 {
			t.Fatalf("DeepSearchInMap() = %v, want %v", got, 8080)
		}
	})

	t.Run("empty path returns shallow copy", func(t *testing.T) {
		t.Parallel()

		m := map[string]interface{}{"key": "value"}
		got, ok := DeepSearchInMap(m).(map[string]interface{})
		if !ok {
			t.Fatalf("DeepSearchInMap() type = %T, want map[string]interface{}", DeepSearchInMap(m))
		}
		assertDeepEqual(t, got, m)

		got["key"] = "changed"
		if m["key"] != "value" {
			t.Fatalf("empty path should return a copy, original mutated to %v", m["key"])
		}
	})
}

func assertDeepEqual(t *testing.T, got, want interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("\n got: %#v\nwant: %#v", got, want)
	}
}
