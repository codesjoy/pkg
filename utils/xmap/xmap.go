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

// Package xmap provides helper functions for map manipulation.
package xmap

import (
	"fmt"
	"reflect"
)

// MergeStringMap recursively merges source maps into dst.
//
// Behavior:
//   - Missing keys in dst are copied from src.
//   - For map[string]interface{} values, merge is recursive.
//   - For map[interface{}]interface{} values, keys are converted to strings before merge.
//   - If value types differ between dst and src, the source value is ignored.
func MergeStringMap(dst map[string]interface{}, src ...map[string]interface{}) {
	for _, item := range src {
		mergeStringMap(dst, item)
	}
}

func mergeStringMap(dest, src map[string]interface{}) {
	for sk, sv := range src {
		tv, ok := dest[sk]
		if !ok {
			dest[sk] = sv
			continue
		}

		svType := reflect.TypeOf(sv)
		tvType := reflect.TypeOf(tv)
		if svType != tvType {
			continue
		}

		switch ttv := tv.(type) {
		case map[interface{}]interface{}:
			tsv := sv.(map[interface{}]interface{})
			ssv := ToMapStringInterface(tsv)
			stv := ToMapStringInterface(ttv)
			mergeStringMap(stv, ssv)
			dest[sk] = stv
		case map[string]interface{}:
			mergeStringMap(ttv, sv.(map[string]interface{}))
			dest[sk] = ttv
		default:
			dest[sk] = sv
		}
	}
}

// ToMapStringInterface converts map[interface{}]interface{} to map[string]interface{}.
func ToMapStringInterface(src map[interface{}]interface{}) map[string]interface{} {
	tgt := make(map[string]interface{}, len(src))
	for k, v := range src {
		tgt[fmt.Sprintf("%v", k)] = v
	}
	return tgt
}

// CoverInterfaceMapToStringMap recursively normalizes nested maps to map[string]interface{}.
func CoverInterfaceMapToStringMap(src map[string]interface{}) {
	for k, v := range src {
		src[k] = normalizeValue(v)
	}
}

func normalizeValue(v interface{}) interface{} {
	switch item := v.(type) {
	case map[interface{}]interface{}:
		normalized := ToMapStringInterface(item)
		CoverInterfaceMapToStringMap(normalized)
		return normalized
	case map[string]interface{}:
		CoverInterfaceMapToStringMap(item)
		return item
	case []interface{}:
		for i := range item {
			item[i] = normalizeValue(item[i])
		}
		return item
	default:
		return v
	}
}

// DeepSearchInMap returns the nested value at paths.
//
// When paths is empty, it returns a shallow copy of m.
// If any intermediate path is missing or non-map, it returns nil.
func DeepSearchInMap(m map[string]interface{}, paths ...string) interface{} {
	if len(paths) == 0 {
		tmp := make(map[string]interface{}, len(m))
		for k, v := range m {
			tmp[k] = v
		}
		return tmp
	}

	tmp := m
	for i, k := range paths {
		v, ok := tmp[k]
		if !ok {
			return nil
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			if i != len(paths)-1 {
				return nil
			}
			return v
		}
		tmp = next
	}
	return tmp
}
