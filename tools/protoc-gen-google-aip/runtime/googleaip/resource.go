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

package googleaip

import (
	"fmt"
	"strings"
)

type segmentKind int

const (
	segmentKindLiteral segmentKind = iota
	segmentKindVariable
)

type patternSegment struct {
	kind  segmentKind
	value string
}

// ResourcePattern is a compiled Google API resource pattern.
type ResourcePattern struct {
	Pattern   string
	segments  []patternSegment
	variables []string
}

// ResourceDescriptor describes one resource type and its patterns.
type ResourceDescriptor struct {
	Type      string
	Plural    string
	Singular  string
	NameField string
	Patterns  []ResourcePattern
}

// ResourceMatch contains the pattern and extracted variables for a matched
// resource name.
type ResourceMatch struct {
	DescriptorType string
	Pattern        string
	Values         map[string]string
}

// ResourceReferenceMetadata stores resource_reference metadata for a field.
type ResourceReferenceMetadata struct {
	Type            string
	ChildType       string
	MessageFullName string
	FieldName       string
}

// MustCompilePattern compiles a pattern and panics on invalid syntax.
func MustCompilePattern(pattern string) ResourcePattern {
	compiled, err := compilePattern(pattern)
	if err != nil {
		panic(err)
	}
	return compiled
}

// Parse matches a resource name against the descriptor's known patterns.
func (d ResourceDescriptor) Parse(name string) (ResourceMatch, error) {
	for _, pattern := range d.Patterns {
		values, ok := pattern.match(name)
		if ok {
			return ResourceMatch{
				DescriptorType: d.Type,
				Pattern:        pattern.Pattern,
				Values:         values,
			}, nil
		}
	}

	return ResourceMatch{}, fmt.Errorf("resource name %q does not match type %q", name, d.Type)
}

// Validate reports whether a name matches one of the descriptor patterns.
func (d ResourceDescriptor) Validate(name string) error {
	_, err := d.Parse(name)
	return err
}

// FormatWithPattern formats one descriptor pattern using the provided values.
func (d ResourceDescriptor) FormatWithPattern(pattern string, values map[string]string) (string, error) {
	for _, compiled := range d.Patterns {
		if compiled.Pattern == pattern {
			return compiled.format(values)
		}
	}

	return "", fmt.Errorf("pattern %q is not registered for type %q", pattern, d.Type)
}

// Clone returns a copy safe for callers to mutate.
func (d ResourceDescriptor) Clone() ResourceDescriptor {
	clone := d
	clone.Patterns = append([]ResourcePattern(nil), d.Patterns...)
	return clone
}

// Variables reports the variables referenced by the pattern.
func (p ResourcePattern) Variables() []string {
	return append([]string(nil), p.variables...)
}

func (p ResourcePattern) match(name string) (map[string]string, bool) {
	if name == "" {
		return nil, false
	}

	parts := strings.Split(name, "/")
	if len(parts) != len(p.segments) {
		return nil, false
	}

	values := make(map[string]string, len(p.variables))
	for i, segment := range p.segments {
		part := parts[i]
		if part == "" {
			return nil, false
		}

		switch segment.kind {
		case segmentKindLiteral:
			if part != segment.value {
				return nil, false
			}
		case segmentKindVariable:
			values[segment.value] = part
		default:
			return nil, false
		}
	}

	return values, true
}

func (p ResourcePattern) format(values map[string]string) (string, error) {
	if len(p.segments) == 0 {
		return "", fmt.Errorf("pattern %q is empty", p.Pattern)
	}

	parts := make([]string, 0, len(p.segments))
	for _, segment := range p.segments {
		switch segment.kind {
		case segmentKindLiteral:
			parts = append(parts, segment.value)
		case segmentKindVariable:
			value, ok := values[segment.value]
			if !ok {
				return "", fmt.Errorf("missing value for variable %q in pattern %q", segment.value, p.Pattern)
			}
			if value == "" {
				return "", fmt.Errorf("value for variable %q in pattern %q must not be empty", segment.value, p.Pattern)
			}
			if strings.Contains(value, "/") {
				return "", fmt.Errorf("value for variable %q in pattern %q must not contain '/'", segment.value, p.Pattern)
			}
			parts = append(parts, value)
		default:
			return "", fmt.Errorf("pattern %q contains unknown segment kind", p.Pattern)
		}
	}

	return strings.Join(parts, "/"), nil
}

func compilePattern(pattern string) (ResourcePattern, error) {
	if pattern == "" {
		return ResourcePattern{}, fmt.Errorf("resource pattern must not be empty")
	}

	rawSegments := strings.Split(pattern, "/")
	segments := make([]patternSegment, 0, len(rawSegments))
	variables := make([]string, 0, len(rawSegments))
	seen := make(map[string]struct{}, len(rawSegments))
	for _, raw := range rawSegments {
		if raw == "" {
			return ResourcePattern{}, fmt.Errorf("resource pattern %q contains an empty path segment", pattern)
		}
		if strings.HasPrefix(raw, "{") || strings.HasSuffix(raw, "}") {
			if !strings.HasPrefix(raw, "{") || !strings.HasSuffix(raw, "}") {
				return ResourcePattern{}, fmt.Errorf("resource pattern %q contains malformed variable segment %q", pattern, raw)
			}
			name := strings.TrimSuffix(strings.TrimPrefix(raw, "{"), "}")
			if name == "" {
				return ResourcePattern{}, fmt.Errorf("resource pattern %q contains an empty variable segment", pattern)
			}
			if strings.Contains(name, "/") || strings.Contains(name, "=") {
				return ResourcePattern{}, fmt.Errorf("resource pattern %q contains unsupported variable syntax %q", pattern, raw)
			}
			if _, ok := seen[name]; ok {
				return ResourcePattern{}, fmt.Errorf("resource pattern %q reuses variable %q", pattern, name)
			}
			seen[name] = struct{}{}
			variables = append(variables, name)
			segments = append(segments, patternSegment{kind: segmentKindVariable, value: name})
			continue
		}
		if strings.Contains(raw, "{") || strings.Contains(raw, "}") {
			return ResourcePattern{}, fmt.Errorf("resource pattern %q contains malformed segment %q", pattern, raw)
		}
		segments = append(segments, patternSegment{kind: segmentKindLiteral, value: raw})
	}

	return ResourcePattern{
		Pattern:   pattern,
		segments:  segments,
		variables: variables,
	}, nil
}
