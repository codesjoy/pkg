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

// Package aipsql contains utilities used to comply with API Improvement
// Proposals (AIPs) from https://google.aip.dev/. This includes
// an AIP-160 filter parser and AIP-132 order by clause parser.
package aipsql

// This file contains a parser for AIP-160 filter expressions.
// The EBNF is at https://google.aip.dev/assets/misc/ebnf-filtering.txt
// The function call syntax is not supported which simplifies the parser.
//
// Implemented EBNF (in terms of lexer tokens):
// filter: [expression];
// expression: sequence {WS AND WS sequence};
// sequence: factor {WS factor};
// factor: term {WS OR WS term};
// term: [NEGATE] simple;
// simple: restriction | composite;
// restriction: comparable [COMPARATOR arg];
// comparable: member;
// member: (TEXT | STRING) {DOT TEXT};
// composite: LPAREN expression RPAREN;
// arg: comparable | composite;
//
// TODO(mwarton): Redo whitespace handling.  There are still some cases (like "- 30")
// 				  which are accepted as valid instead of being rejected.
import (
	"fmt"
	"strconv"
)

// ParseFilter parse an AIP-160 filter string into an AST.
func ParseFilter(filter string) (*Filter, error) {
	return newParser(filter).filter()
}

type parser struct {
	lexer filterLexer
}

func newParser(input string) *parser {
	return &parser{lexer: *newLexer(input)}
}

func (p *parser) expect(kind string) error {
	t, err := p.lexer.Peek()
	if err != nil {
		return err
	}
	if t.kind != kind {
		return fmt.Errorf("expected %s but got %s(%q)", kind, t.kind, t.value)
	}
	_, err = p.lexer.Next()
	return err
}

func (p *parser) accept(kind string) (*token, error) {
	t, err := p.lexer.Peek()
	if err != nil {
		return nil, err
	}
	if t.kind != kind {
		return nil, nil
	}
	return p.lexer.Next()
}

func (p *parser) filter() (*Filter, error) {
	t, err := p.accept(kindEnd)
	if err != nil {
		return nil, err
	}
	if t != nil {
		return &Filter{}, nil
	}
	e, err := p.expression()
	if err != nil {
		return nil, err
	}
	return &Filter{Expression: e}, p.expect(kindEnd)
}

func (p *parser) expression() (*Expression, error) {
	s, err := p.sequence()
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, nil
	}
	e := &Expression{}
	e.Sequences = append(e.Sequences, s)
	for {
		and, err := p.accept(kindAnd)
		if err != nil {
			return nil, err
		}
		if and == nil {
			break
		}
		s, err := p.sequence()
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, fmt.Errorf("expected sequence after AND")
		}
		e.Sequences = append(e.Sequences, s)
	}
	return e, nil
}

func (p *parser) sequence() (*Sequence, error) {
	s := &Sequence{}
	for {
		f, err := p.factor()
		if err != nil {
			return nil, err
		}
		if f == nil {
			break
		}
		s.Factors = append(s.Factors, f)
	}
	if len(s.Factors) == 0 {
		return nil, nil
	}
	return s, nil
}

func (p *parser) factor() (*Factor, error) {
	t, err := p.term()
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	f := &Factor{}
	f.Terms = append(f.Terms, t)
	for {
		or, err := p.accept(kindOr)
		if err != nil {
			return nil, err
		}
		if or == nil {
			break
		}
		t, err := p.term()
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, fmt.Errorf("expected term after OR")
		}
		f.Terms = append(f.Terms, t)
	}
	return f, nil
}

func (p *parser) term() (*Term, error) {
	n, err := p.accept(kindNegate)
	if err != nil {
		return nil, err
	}
	s, err := p.simple()
	if err != nil {
		return nil, err
	}
	if s == nil {
		if n != nil {
			return nil, fmt.Errorf("expected simple term after negation %q", n.value)
		}
		return nil, nil
	}
	return &Term{Negated: n != nil, Simple: s}, nil
}

func (p *parser) simple() (*Simple, error) {
	r, err := p.restriction()
	if err != nil {
		return nil, err
	}
	if r != nil {
		return &Simple{Restriction: r}, nil
	}
	c, err := p.composite()
	if err != nil {
		return nil, err
	}
	if c != nil {
		return &Simple{Composite: c}, nil
	}
	return nil, nil
}

func (p *parser) restriction() (*Restriction, error) {
	comparable, err := p.comparable()
	if err != nil {
		return nil, err
	}
	if comparable == nil {
		return nil, nil
	}
	comparator, err := p.accept(kindComparator)
	if err != nil {
		return nil, err
	}
	if comparator == nil {
		return &Restriction{Comparable: comparable}, nil
	}
	arg, err := p.arg()
	if err != nil {
		return nil, err
	}
	if arg == nil {
		return nil, fmt.Errorf("expected arg after %s", comparator.value)
	}
	return &Restriction{Comparable: comparable, Comparator: comparator.value, Arg: arg}, nil
}

func (p *parser) comparable() (*Comparable, error) {
	m, err := p.member()
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	return &Comparable{Member: m}, nil
}

func (p *parser) member() (*Member, error) {
	v, err := p.accept(kindString)
	if err != nil {
		return nil, err
	}
	if v != nil {
		v.value, err = strconv.Unquote(v.value)
		if err != nil {
			return nil, fmt.Errorf("error unquoting string: %w", err)
		}
		return &Member{Value: v.value}, nil
	}

	v, err = p.accept(kindText)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	m := &Member{Value: v.value}
	for {
		dot, err := p.accept(kindDot)
		if err != nil {
			return nil, err
		}
		if dot == nil {
			break
		}
		f, err := p.accept(kindText)
		if err != nil {
			return nil, err
		}
		if f == nil {
			return nil, fmt.Errorf("expected field name after '.'")
		}
		m.Fields = append(m.Fields, f.value)
	}
	return m, nil
}

func (p *parser) composite() (*Expression, error) {
	lparen, err := p.accept(kindLParen)
	if err != nil {
		return nil, err
	}
	if lparen == nil {
		return nil, nil
	}
	e, err := p.expression()
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("expected expression")
	}
	return e, p.expect(kindRParen)
}

func (p *parser) arg() (*Arg, error) {
	comparable, err := p.comparable()
	if err != nil {
		return nil, err
	}
	if comparable != nil {
		return &Arg{Comparable: comparable}, nil
	}
	composite, err := p.composite()
	if err != nil {
		return nil, err
	}
	if composite != nil {
		return &Arg{Composite: composite}, nil
	}
	return nil, nil
}
