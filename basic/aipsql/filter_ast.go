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
	"strconv"
	"strings"
)

// AST Nodes.  These are based on the EBNF at https://google.aip.dev/assets/misc/ebnf-filtering.txt
// Note that the syntax for functions is not currently supported.
//
// The AST models the AIP-160 expression grammar with the following precedence
// (highest to lowest):
//
//   1. Parenthesized groups (composite expressions)
//   2. NOT / negation (unary -)
//   3. OR (disjunction, Factor)
//   4. whitespace/implicit AND (sequence)
//   5. AND (conjunction, Expression)
//
// Example: `a = 1 AND b = 2 OR c = 3 NOT d = 4`
// parses as: AND(Sequence(Factor(OR(a=1, AND(Sequence(Factor(b=2), Factor(OR(c=3, NOT(d=4)))))))))

// Filter is a possibly empty filter expression.
type Filter struct {
	Expression *Expression // Optional, may be nil.
}

func (v *Filter) String() string {
	var s strings.Builder
	s.WriteString("filter{")
	if v.Expression != nil {
		s.WriteString(v.Expression.String())
	}
	s.WriteString("}")
	return s.String()
}

// Expression may either be a conjunction (AND) of sequences or a simple
// sequence.
//
// Note, the AND is case-sensitive.
//
// Example: `a b AND c AND d`
//
// The expression `(a b) AND c AND d` is equivalent to the example.
type Expression struct {
	// Sequences are always joined by an AND operator
	Sequences []*Sequence
}

func (v *Expression) String() string {
	var s strings.Builder
	s.WriteString("expression{")
	for i, c := range v.Sequences {
		if i > 0 {
			s.WriteString(",")
		}
		if c != nil {
			s.WriteString(c.String())
		}
	}
	s.WriteString("}")
	return s.String()
}

// Sequence is composed of one or more whitespace (WS) separated factors.
//
// A sequence expresses a logical relationship between 'factors' where
// the ranking of a filter result may be scored according to the number
// factors that match and other such criteria as the proximity of factors
// to each other within a document.
//
// When filters are used with exact match semantics rather than fuzzy
// match semantics, a sequence is equivalent to AND.
//
// Example: `New York Giants OR Yankees`
//
// The expression `New York (Giants OR Yankees)` is equivalent to the
// example.
type Sequence struct {
	// Factors are always joined by an (implicit) AND operator
	Factors []*Factor
}

func (v *Sequence) String() string {
	var s strings.Builder
	s.WriteString("sequence{")
	for i, c := range v.Factors {
		if i > 0 {
			s.WriteString(",")
		}
		if c != nil {
			s.WriteString(c.String())
		}
	}
	s.WriteString("}")
	return s.String()
}

// Factor may either be a disjunction (OR) of terms or a simple term.
//
// Note, the OR is case-sensitive.
//
// Example: `a < 10 OR a >= 100`
type Factor struct {
	// Terms are always joined by an OR operator
	Terms []*Term
}

func (v *Factor) String() string {
	var s strings.Builder
	s.WriteString("factor{")
	for i, c := range v.Terms {
		if i > 0 {
			s.WriteString(",")
		}
		if c != nil {
			s.WriteString(c.String())
		}
	}
	s.WriteString("}")
	return s.String()
}

// Term may either be unary or simple expressions.
//
// Unary expressions negate the simple expression, either mathematically `-`
// or logically `NOT`. The negation styles may be used interchangeably.
//
// Note, the `NOT` is case-sensitive and must be followed by at least one
// whitespace (WS).
//
// Examples:
// * logical not     : `NOT (a OR b)`
// * alternative not : `-file:".java"`
// * negation        : `-30`
type Term struct {
	Negated bool
	Simple  *Simple
}

func (v *Term) String() string {
	var s strings.Builder
	s.WriteString("term{")
	if v.Negated {
		s.WriteString("-")
	}
	if v.Simple != nil {
		s.WriteString(v.Simple.String())
	}
	s.WriteString("}")
	return s.String()
}

// Simple expressions may either be a restriction or a nested (composite)
// expression.
type Simple struct {
	Restriction *Restriction
	// Composite is a parenthesized expression, commonly used to group
	// terms or clarify operator precedence.
	//
	// Example: `(msg.endsWith('world') AND retries < 10)`
	Composite *Expression
}

func (v *Simple) String() string {
	var s strings.Builder
	s.WriteString("simple{")
	if v.Restriction != nil {
		s.WriteString(v.Restriction.String())
	}
	if v.Restriction != nil && v.Composite != nil {
		s.WriteString(",")
	}
	if v.Composite != nil {
		s.WriteString(v.Composite.String())
	}
	s.WriteString("}")
	return s.String()
}

// Restriction expresses a relationship between a comparable value and a
// single argument. When the restriction only specifies a comparable
// without an operator, this is a global restriction.
//
// Note, restrictions are not whitespace sensitive.
//
// Examples:
// * equality         : `package=com.google`
// * inequality       : `msg != 'hello'`
// * greater than     : `1 > 0`
// * greater or equal : `2.5 >= 2.4`
// * less than        : `yesterday < request.time`
// * less or equal    : `experiment.rollout <= cohort(request.user)`
// * has              : `map:key`
// * global           : `prod`
//
// In addition to the global, equality, and ordering operators, filters
// also support the has (`:`) operator. The has operator is unique in
// that it can test for presence or value based on the proto3 type of
// the `comparable` value. The has operator is useful for validating the
// structure and contents of complex values.
type Restriction struct {
	Comparable *Comparable
	// Comparators supported by list filters: <=, <. >=, >, !=, =, :
	Comparator string
	Arg        *Arg
}

func (v *Restriction) String() string {
	var s strings.Builder
	s.WriteString("restriction{")
	if v.Comparable != nil {
		s.WriteString(v.Comparable.String())
	}
	if v.Comparator != "" {
		s.WriteString(",")
		s.WriteString(strconv.Quote(v.Comparator))
	}
	if v.Arg != nil {
		s.WriteString(",")
		s.WriteString(v.Arg.String())
	}
	s.WriteString("}")
	return s.String()
}

// Arg is the right-hand side of a restriction.
type Arg struct {
	Comparable *Comparable
	// Composite is a parenthesized expression, commonly used to group
	// terms or clarify operator precedence.
	//
	// Example: `(msg.endsWith('world') AND retries < 10)`
	Composite *Expression
}

func (v *Arg) String() string {
	var s strings.Builder
	s.WriteString("arg{")
	if v.Comparable != nil {
		s.WriteString(v.Comparable.String())
	}
	if v.Comparable != nil && v.Composite != nil {
		s.WriteString(",")
	}
	if v.Composite != nil {
		s.WriteString(v.Composite.String())
	}
	s.WriteString("}")
	return s.String()
}

// Comparable may either be a member or function.  As functions are not currently supported, it is always a member.
type Comparable struct {
	Member *Member
}

func (v *Comparable) String() string {
	var s strings.Builder
	s.WriteString("comparable{")
	if v.Member != nil {
		s.WriteString(v.Member.String())
	}
	s.WriteString("}")
	return s.String()
}

// Member expressions are either value or DOT qualified field references.
//
// Example: `expr.type_map.1.type`
type Member struct {
	Value  string
	Fields []string
}

func (v *Member) String() string {
	var s strings.Builder
	s.WriteString("member{")
	s.Write([]byte(strconv.Quote(v.Value)))
	if len(v.Fields) > 0 {
		s.WriteString(", {")
	}
	for i, c := range v.Fields {
		if i > 0 {
			s.WriteString(",")
		}
		s.WriteString(strconv.Quote(c))
	}
	s.WriteString("}}")
	return s.String()
}
