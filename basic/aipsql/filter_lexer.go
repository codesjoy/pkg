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
	// kindComparator represents comparison operators: <=, >=, !=, <, >, =, :
	kindComparator = "COMPARATOR"
	// kindNegate represents the negation operator: NOT or -
	kindNegate = "NEGATE"
	// kindAnd represents the logical AND keyword (case-sensitive)
	kindAnd = "AND"
	// kindOr represents the logical OR keyword (case-sensitive)
	kindOr = "OR"
	// kindDot represents the field traversal operator (.)
	kindDot = "DOT"
	// kindLParen represents the left parenthesis
	kindLParen = "LPAREN"
	// kindRParen represents the right parenthesis
	kindRParen = "RPAREN"
	// kindComma represents the comma separator
	kindComma = "COMMA"
	// kindString represents a double-quoted string literal
	kindString = "STRING"
	// kindText represents an unquoted identifier or value
	kindText = "TEXT"
	// kindEnd marks the end of input
	kindEnd = "END"
)

// token represents a single lexical token produced by the filter lexer.
type token struct {
	kind  string
	value string
}

// filterLexer tokenizes an AIP-160 filter expression into a stream of tokens.
// It supports single-token lookahead via the next field.
type filterLexer struct {
	input string
	next  *token
}

// newLexer creates a lexer for an AIP-160 filter expression.
func newLexer(input string) *filterLexer {
	return &filterLexer{input: input}
}

// Peek returns the next token without consuming it. The same token is returned
// on subsequent Peek or Next calls until consumed.
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

// Next consumes and returns the next token from the input.
// Tokenization priority order (first match wins):
//  1. Two-character comparators (<=, >=, !=)
//  2. Single-character tokens (<, >, =, :, -, ., (, ), ,)
//  3. Double-quoted string literals
//  4. Keyword tokens: NOT, AND, OR (require trailing whitespace to distinguish from text)
//  5. Unquoted text (runs until a separator character)
func (l *filterLexer) Next() (*token, error) {
	// Return the previously peeked token if available.
	if l.next != nil {
		next := l.next
		l.next = nil
		return next, nil
	}

	// Skip leading whitespace before deciding the next token type.
	l.input = trimFilterLeadingWhitespace(l.input)
	if len(l.input) == 0 {
		return &token{kind: kindEnd}, nil
	}

	input := l.input

	// Priority 1: Two-character comparators must be checked before single-char.
	if comparator, ok := twoCharComparatorToken(input); ok {
		l.input = input[2:]
		return comparator, nil
	}

	// Priority 2: Single-character tokens.
	if single, ok := singleCharToken(input[0]); ok {
		l.input = input[1:]
		return single, nil
	}

	// Priority 3: Double-quoted string literals.
	if input[0] == '"' {
		stringTokenLength, ok := filterStringTokenLength(input)
		if !ok {
			return nil, fmt.Errorf("error: unable to lex token from %q", input)
		}
		value := input[:stringTokenLength]
		l.input = input[stringTokenLength:]
		return &token{kind: kindString, value: value}, nil
	}

	// Priority 4: Keyword tokens (NOT, AND, OR). These must be followed by
	// whitespace so that e.g. "ANDROID" is lexed as TEXT, not AND + ROID.
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

	// Priority 5: Unquoted text — consume until a separator character.
	textTokenLength := filterTextTokenLength(input)
	if textTokenLength == 0 {
		return nil, fmt.Errorf("error: unable to lex token from %q", input)
	}
	value := input[:textTokenLength]
	l.input = input[textTokenLength:]
	return &token{kind: kindText, value: value}, nil
}

// twoCharComparatorToken checks whether the input starts with a two-character
// comparator operator (<=, >=, or !=).
func twoCharComparatorToken(input string) (*token, bool) {
	if len(input) < 2 {
		return nil, false
	}
	switch input[:2] {
	case "<=", ">=", "!=":
		return &token{kind: kindComparator, value: input[:2]}, true
	default:
		return nil, false
	}
}

// singleCharToken maps a single byte to its token kind. Covers comparators (<, >, =, :),
// negation (-), dot traversal (.), parentheses, and comma.
func singleCharToken(ch byte) (*token, bool) {
	switch ch {
	case '<', '>', '=', ':':
		return &token{kind: kindComparator, value: string(ch)}, true
	case '-':
		return &token{kind: kindNegate, value: "-"}, true
	case '.':
		return &token{kind: kindDot, value: "."}, true
	case '(':
		return &token{kind: kindLParen, value: "("}, true
	case ')':
		return &token{kind: kindRParen, value: ")"}, true
	case ',':
		return &token{kind: kindComma, value: ","}, true
	default:
		return nil, false
	}
}

// trimFilterLeadingWhitespace removes leading spaces, tabs, carriage returns, and newlines.
func trimFilterLeadingWhitespace(input string) string {
	for len(input) > 0 && isFilterWhitespace(input[0]) {
		input = input[1:]
	}
	return input
}

// hasFilterKeywordToken returns true if input starts with the given keyword followed by
// at least one whitespace character. This prevents keywords like "AND" from matching
// the start of "ANDROID".
func hasFilterKeywordToken(input, keyword string) bool {
	if len(input) <= len(keyword) || input[:len(keyword)] != keyword {
		return false
	}
	return isFilterWhitespace(input[len(keyword)])
}

// filterStringTokenLength scans a double-quoted string literal and returns its length
// in bytes (including the surrounding quotes). Handles backslash escapes.
// Returns (0, false) if the string is unterminated.
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

// filterTextTokenLength returns the length of the longest unquoted text prefix.
// Text is terminated by any whitespace or separator character (., <, >, =, !, :, (, ), ,).
func filterTextTokenLength(input string) int {
	for i := 0; i < len(input); i++ {
		if isFilterTextSeparator(input[i]) {
			return i
		}
	}
	return len(input)
}

// isFilterWhitespace returns true for space, tab, carriage return, and newline.
func isFilterWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

// isFilterTextSeparator returns true for characters that terminate an unquoted text token.
// This includes whitespace and all operator/punctuation characters.
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
