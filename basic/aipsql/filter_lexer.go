// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package aipsql

import (
	"fmt"
)

const (
	kindComparator = "COMPARATOR"
	kindNegate     = "NEGATE"
	kindAnd        = "AND"
	kindOr         = "OR"
	kindDot        = "DOT"
	kindLParen     = "LPAREN"
	kindRParen     = "RPAREN"
	kindComma      = "COMMA"
	kindString     = "STRING"
	kindText       = "TEXT"
	kindEnd        = "END"
)

type token struct {
	kind  string
	value string
}

type filterLexer struct {
	input string
	next  *token
}

// newLexer creates a lexer for an AIP-160 filter expression.
func newLexer(input string) *filterLexer {
	return &filterLexer{input: input}
}

func (l *filterLexer) Peek() (*token, error) {
	if l.next == nil {
		var err error
		l.next, err = l.Next()
		if err != nil {
			return nil, err
		}
	}
	return l.next, nil
}

func (l *filterLexer) Next() (*token, error) {
	if l.next != nil {
		next := l.next
		l.next = nil
		return next, nil
	}
	l.input = trimFilterLeadingWhitespace(l.input)
	if len(l.input) == 0 {
		return &token{kind: kindEnd}, nil
	}
	input := l.input
	switch {
	case len(input) >= 2 && input[:2] == "<=":
		l.input = input[2:]
		return &token{kind: kindComparator, value: "<="}, nil
	case len(input) >= 2 && input[:2] == ">=":
		l.input = input[2:]
		return &token{kind: kindComparator, value: ">="}, nil
	case len(input) >= 2 && input[:2] == "!=":
		l.input = input[2:]
		return &token{kind: kindComparator, value: "!="}, nil
	}

	switch input[0] {
	case '<', '>', '=', ':':
		l.input = input[1:]
		return &token{kind: kindComparator, value: input[:1]}, nil
	case '-':
		l.input = input[1:]
		return &token{kind: kindNegate, value: "-"}, nil
	case '.':
		l.input = input[1:]
		return &token{kind: kindDot, value: "."}, nil
	case '(':
		l.input = input[1:]
		return &token{kind: kindLParen, value: "("}, nil
	case ')':
		l.input = input[1:]
		return &token{kind: kindRParen, value: ")"}, nil
	case ',':
		l.input = input[1:]
		return &token{kind: kindComma, value: ","}, nil
	case '"':
		stringTokenLength, ok := filterStringTokenLength(input)
		if !ok {
			return nil, fmt.Errorf("error: unable to lex token from %q", input)
		}
		value := input[:stringTokenLength]
		l.input = input[stringTokenLength:]
		return &token{kind: kindString, value: value}, nil
	}

	if hasFilterKeywordToken(input, "NOT") {
		l.input = input[4:]
		return &token{kind: kindNegate, value: "NOT"}, nil
	}
	if hasFilterKeywordToken(input, "AND") {
		l.input = input[4:]
		return &token{kind: kindAnd, value: "AND"}, nil
	}
	if hasFilterKeywordToken(input, "OR") {
		l.input = input[3:]
		return &token{kind: kindOr, value: "OR"}, nil
	}

	textTokenLength := filterTextTokenLength(input)
	if textTokenLength == 0 {
		return nil, fmt.Errorf("error: unable to lex token from %q", input)
	}
	value := input[:textTokenLength]
	l.input = input[textTokenLength:]
	return &token{kind: kindText, value: value}, nil
}

func trimFilterLeadingWhitespace(input string) string {
	for len(input) > 0 && isFilterWhitespace(input[0]) {
		input = input[1:]
	}
	return input
}

func hasFilterKeywordToken(input, keyword string) bool {
	if len(input) <= len(keyword) || input[:len(keyword)] != keyword {
		return false
	}
	return isFilterWhitespace(input[len(keyword)])
}

func filterStringTokenLength(input string) (int, bool) {
	for i := 1; i < len(input); i++ {
		switch input[i] {
		case '\\':
			i++
			if i >= len(input) {
				return 0, false
			}
		case '"':
			return i + 1, true
		}
	}
	return 0, false
}

func filterTextTokenLength(input string) int {
	for i := 0; i < len(input); i++ {
		if isFilterTextSeparator(input[i]) {
			return i
		}
	}
	return len(input)
}

func isFilterWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

func isFilterTextSeparator(ch byte) bool {
	if isFilterWhitespace(ch) {
		return true
	}
	switch ch {
	case '.', ',', '<', '>', '=', '!', ':', '(', ')':
		return true
	default:
		return false
	}
}
