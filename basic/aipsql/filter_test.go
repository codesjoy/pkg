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
	"testing"

	"github.com/codesjoy/pkg/basic/aipsql/testing/assertions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Filter Parser Tests
// ============================================================================

func TestTokenKinds(t *testing.T) {
	tests := []struct {
		input string
		kind  string
		value string
	}{
		{input: "<= 10", kind: kindComparator, value: "<="},
		{input: "-file", kind: kindNegate, value: "-"},
		{input: "NOT file", kind: kindNegate, value: "NOT"},
		{input: "AND b", kind: kindAnd, value: "AND"},
		{input: "OR a", kind: kindOr, value: "OR"},
		{input: ".field", kind: kindDot, value: "."},
		{input: "(arg)", kind: kindLParen, value: "("},
		{input: ")", kind: kindRParen, value: ")"},
		{input: ", arg2)", kind: kindComma, value: ","},
		{input: "text", kind: kindText, value: "text"},
		{input: "\"string\"", kind: kindString, value: "\"string\""},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			token, err := newLexer(test.input).Next()
			if err != nil {
				t.Fatalf("Error when lexing: %v", err)
			}
			if token.kind != test.kind {
				t.Errorf("Wrong kind: got %s, want %s", token.kind, test.kind)
			}
			if token.value != test.value {
				t.Errorf("Wrong kind: got %s, want %s", token.value, test.value)
			}
		})
	}
}

func TestWhitespaceLexing(t *testing.T) {
	filter := "text \"string with whitespace\" (43 AND 44) OR 45 NOT function(arg1, arg2):hello -field1.field2: hello field < 36"
	tokens := []token{
		{kind: kindText, value: "text"},
		{kind: kindString, value: "\"string with whitespace\""},
		{kind: kindLParen, value: "("},
		{kind: kindText, value: "43"},
		{kind: kindAnd, value: "AND"},
		{kind: kindText, value: "44"},
		{kind: kindRParen, value: ")"},
		{kind: kindOr, value: "OR"},
		{kind: kindText, value: "45"},
		{kind: kindNegate, value: "NOT"},
		{kind: kindText, value: "function"},
		{kind: kindLParen, value: "("},
		{kind: kindText, value: "arg1"},
		{kind: kindComma, value: ","},
		{kind: kindText, value: "arg2"},
		{kind: kindRParen, value: ")"},
		{kind: kindComparator, value: ":"},
		{kind: kindText, value: "hello"},
		{kind: kindNegate, value: "-"},
		{kind: kindText, value: "field1"},
		{kind: kindDot, value: "."},
		{kind: kindText, value: "field2"},
		{kind: kindComparator, value: ":"},
		{kind: kindText, value: "hello"},
		{kind: kindText, value: "field"},
		{kind: kindComparator, value: "<"},
		{kind: kindText, value: "36"},
		{kind: kindEnd, value: ""},
		{kind: kindEnd, value: ""},
	}
	l := newLexer(filter)
	for i, expected := range tokens {
		actual, err := l.Next()
		if err != nil {
			t.Fatalf("Error getting next token: %v", err)
		}
		if actual.kind != expected.kind {
			t.Errorf(
				"wrong token kind for token %d: got %s, want %s",
				i,
				actual.kind,
				expected.kind,
			)
		}
		if actual.value != expected.value {
			t.Errorf(
				"wrong token value for token %d: got %s, want %s",
				i,
				actual.value,
				expected.value,
			)
		}
	}
}

func TestFullParse(t *testing.T) {
	tests := []struct {
		input     string
		ast       string
		expectErr bool
	}{
		{input: "", ast: "filter{}"},
		{input: " ", ast: "filter{}"},
		{
			input: "simple",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"simple\"}}}}}}}}}}",
		},
		{
			input: " wsBefore",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"wsBefore\"}}}}}}}}}}",
		},
		{
			input: "wsAfter ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"wsAfter\"}}}}}}}}}}",
		},
		{
			input: " wsAround ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"wsAround\"}}}}}}}}}}",
		},
		{
			input: "\"string\"",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"string\"}}}}}}}}}}",
		},
		{
			input: " \"string\" ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"string\"}}}}}}}}}}",
		},
		{
			input: "\"ws string\"",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"ws string\"}}}}}}}}}}",
		},
		{
			input: "-negated",
			ast:   "filter{expression{sequence{factor{term{-simple{restriction{comparable{member{\"negated\"}}}}}}}}}}",
		},
		{
			input: " - negated ",
			ast:   "filter{expression{sequence{factor{term{-simple{restriction{comparable{member{\"negated\"}}}}}}}}}}",
		},
		// This is a common case (lots of test names are separated by -).
		{
			input: "dash-separated-name",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"dash-separated-name\"}}}}}}}}}}",
		},
		{
			input: "term -negated-term",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"term\"}}}}}}},factor{term{-simple{restriction{comparable{member{\"negated-term\"}}}}}}}}}}",
		},
		{
			input: "NOT negated",
			ast:   "filter{expression{sequence{factor{term{-simple{restriction{comparable{member{\"negated\"}}}}}}}}}}",
		},
		{
			input: " NOT negated ",
			ast:   "filter{expression{sequence{factor{term{-simple{restriction{comparable{member{\"negated\"}}}}}}}}}}",
		},
		{
			input: " NOTnegated ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"NOTnegated\"}}}}}}}}}}",
		},
		{
			input: "implicit and",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"implicit\"}}}}}}},factor{term{simple{restriction{comparable{member{\"and\"}}}}}}}}}}",
		},
		{
			input: " implicit and ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"implicit\"}}}}}}},factor{term{simple{restriction{comparable{member{\"and\"}}}}}}}}}}",
		},
		{
			input: "explicit AND and",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"explicit\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{\"and\"}}}}}}}}}}",
		},
		{input: "explicit AND ", expectErr: true},
		{
			input: "explicit AND and",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"explicit\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{\"and\"}}}}}}}}}}",
		},
		{
			input: " explicit AND and ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"explicit\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{\"and\"}}}}}}}}}}",
		},
		{
			input: " explicit ANDnotand ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"explicit\"}}}}}}},factor{term{simple{restriction{comparable{member{\"ANDnotand\"}}}}}}}}}}",
		},
		{
			input: "test OR or",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"test\"}}}}}},term{simple{restriction{comparable{member{\"or\"}}}}}}}}}}",
		},
		{input: "test OR ", expectErr: true},
		{
			input: "test ORnotor",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"test\"}}}}}}},factor{term{simple{restriction{comparable{member{\"ORnotor\"}}}}}}}}}}",
		},
		{
			input: " test OR or ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"test\"}}}}}},term{simple{restriction{comparable{member{\"or\"}}}}}}}}}}",
		},
		{
			input: " testORor ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"testORor\"}}}}}}}}}}",
		},
		{
			input: "implicit and AND explicit",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"implicit\"}}}}}}},factor{term{simple{restriction{comparable{member{\"and\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{\"explicit\"}}}}}}}}}}",
		},
		{
			input: "implicit with OR term AND explicit OR term",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"implicit\"}}}}}}},factor{term{simple{restriction{comparable{member{\"with\"}}}}}},term{simple{restriction{comparable{member{\"term\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{\"explicit\"}}}}}},term{simple{restriction{comparable{member{\"term\"}}}}}}}}}}",
		},
		{
			input: "(composite)",
			ast:   "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{\"composite\"}}}}}}}}}}}}}}}",
		},
		{
			input: " (composite) ",
			ast:   "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{\"composite\"}}}}}}}}}}}}}}}",
		},
		{
			input: "( composite )",
			ast:   "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{\"composite\"}}}}}}}}}}}}}}}",
		},
		{
			input: " ( composite ) ",
			ast:   "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{\"composite\"}}}}}}}}}}}}}}}",
		},
		{
			input: " ( composite multi) ",
			ast:   "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{\"composite\"}}}}}}},factor{term{simple{restriction{comparable{member{\"multi\"}}}}}}}}}}}}}}}",
		},
		{
			input: "value<21",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\"<\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: "value < 21",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\"<\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: " value < 21 ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\"<\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: "value<=21",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\"<=\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: "value>21",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\">\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: "value>=21",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\">=\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: "value=21",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\"=\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: "value!=21",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\"!=\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: "value:21",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\":\",arg{comparable{member{\"21\"}}}}}}}}}}}",
		},
		{
			input: "value=(composite)",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"value\"}}},\"=\",arg{expression{sequence{factor{term{simple{restriction{comparable{member{\"composite\"}}}}}}}}}}}}}}}}}",
		},
		// Note: although this parses correctly as a "global" restriction, the implementation doesn't handle this type of restriction, so an error will be returned higher in the stack.
		{
			input: "member.field",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"member\", {\"field\"}}}}}}}}}}",
		},
		{
			input: " member.field > 4 ",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"member\", {\"field\"}}},\">\",arg{comparable{member{\"4\"}}}}}}}}}}}",
		},
		{
			input: "composite (expression)",
			ast:   "filter{expression{sequence{factor{term{simple{restriction{comparable{member{\"composite\"}}}}}}},factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{\"expression\"}}}}}}}}}}}}}}}",
		},
		// This should parse as a function, but function parsing is not implemented.
		// {input: "function(expression)", ast: ""},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			filter, err := ParseFilter(test.input)
			if test.expectErr {
				if err == nil {
					t.Fatalf(
						"expected error but no error produced from input: %q\nparsed as:%q",
						test.input,
						filter.String(),
					)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ast := filter.String()
			if ast != test.ast {
				t.Errorf(
					"incorrect AST parsed from input %q:\ngot %q\nwant %q",
					test.input,
					ast,
					test.ast,
				)
			}
		})
	}
}

// ============================================================================
// Filter Generator Tests
// ============================================================================

func TestWhereClause(t *testing.T) {
	subFunc := func(sub string) string {
		if sub == "somevalue" {
			return "somevalue-v2"
		}
		return sub
	}
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("foo").
			WithDatabaseName("db_foo").
			FilterableImplicitly().
			Build(),
		NewColumn().WithFieldPath("bar").
			WithDatabaseName("db_bar").
			FilterableImplicitly().
			Build(),
		NewColumn().WithFieldPath("baz").WithDatabaseName("db_baz").Filterable().Build(),
		NewColumn().WithFieldPath("kv").
			WithDatabaseName("db_kv").
			KeyValue().
			Filterable().
			Build(),
		NewColumn().WithFieldPath("bool").
			WithDatabaseName("db_bool").
			Bool().
			Filterable().
			Build(),
		NewColumn().WithFieldPath("unfilterable").WithDatabaseName("unfilterable").Build(),
		NewColumn().WithFieldPath("qux").
			WithDatabaseName("db_qux").
			WithArgumentSubstitutor(subFunc).
			Filterable().
			Build(),
		NewColumn().WithFieldPath("quux").
			WithDatabaseName("db_quux").
			WithArgumentSubstitutor(subFunc).
			Filterable().
			KeyValue().
			Build(),
	).
		Build()

	t.Run("Empty filter", func(t *testing.T) {
		result, pars, err := table.WhereClause(&Filter{}, "p_")
		require.NoError(t, err)
		assert.Equal(t, 0, len(pars))
		assert.Equal(t, "(TRUE)", result)
	})

	t.Run("Simple filter", func(t *testing.T) {
		t.Run("has operator", func(t *testing.T) {
			filter, err := ParseFilter("foo:somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "%somevalue%",
				},
			}, pars)
			assert.Equal(t, "(db_foo LIKE @p_0)", result)
		})

		t.Run("equals operator", func(t *testing.T) {
			filter, err := ParseFilter("foo = somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "somevalue",
				},
			}, pars)
			assert.Equal(t, "(db_foo = @p_0)", result)
		})

		t.Run("equals operator on bool column", func(t *testing.T) {
			filter, err := ParseFilter("bool = true AND bool = false")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Empty(t, pars) // Boolean values don't need parameters
			assert.Equal(t, "((db_bool = TRUE) AND (db_bool = FALSE))", result)
		})

		t.Run("range operator", func(t *testing.T) {
			filter, err := ParseFilter("foo > somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "somevalue",
				},
			}, pars)
			assert.Equal(t, "(db_foo > @p_0)", result)
		})

		t.Run("range operator on bool column is rejected", func(t *testing.T) {
			filter, err := ParseFilter("bool > true")
			require.NoError(t, err)

			_, _, err = table.WhereClause(filter, "p_")
			assertions.ErrLike(
				t,
				err,
				[]any{`comparator ">" is not supported for boolean field "bool"`},
			)
		})

		t.Run("not equals operator", func(t *testing.T) {
			filter, err := ParseFilter("foo != somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "somevalue",
				},
			}, pars)
			assert.Equal(t, "(db_foo <> @p_0)", result)
		})

		t.Run("not equals operator on bool column", func(t *testing.T) {
			filter, err := ParseFilter("bool != true AND bool != false")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Empty(t, pars) // Boolean values don't need parameters
			assert.Equal(t, "((db_bool <> TRUE) AND (db_bool <> FALSE))", result)
		})

		t.Run("implicit match operator", func(t *testing.T) {
			filter, err := ParseFilter("somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "%somevalue%",
				},
			}, pars)
			assert.Equal(t, "(db_foo LIKE @p_0 OR db_bar LIKE @p_0)", result)
		})

		t.Run("key value contains operator", func(t *testing.T) {
			filter, err := ParseFilter("kv.key:somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "key",
				},
				{
					Name:  "p_1",
					Value: "%somevalue%",
				},
			}, pars)
			assert.Equal(
				t,
				"(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value LIKE @p_1))",
				result,
			)
		})

		t.Run("key value equal operator", func(t *testing.T) {
			filter, err := ParseFilter("kv.key=somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "key",
				},
				{
					Name:  "p_1",
					Value: "somevalue",
				},
			}, pars)
			assert.Equal(t,
				"(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value = @p_1))",
				result,
			)
		})

		t.Run("key value not equal operator", func(t *testing.T) {
			filter, err := ParseFilter("kv.key!=somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "key",
				},
				{
					Name:  "p_1",
					Value: "somevalue",
				},
			}, pars)
			assert.Equal(
				t,
				"(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value <> @p_1))",
				result,
			)
		})

		t.Run("key value missing key contains operator", func(t *testing.T) {
			filter, err := ParseFilter("kv:somevalue")
			require.NoError(t, err)

			_, _, err = table.WhereClause(filter, "p_")
			assertions.ErrLike(t, err, []any{"key value columns must specify the key to search on"})
		})

		t.Run("unsupported composite to LIKE", func(t *testing.T) {
			filter, err := ParseFilter("foo:(somevalue)")
			require.NoError(t, err)

			_, _, err = table.WhereClause(filter, "p_")
			assertions.ErrLike(
				t,
				err,
				[]any{"composite expressions are not allowed as RHS to has (:) operator"},
			)
		})

		t.Run("unsupported composite to equals", func(t *testing.T) {
			filter, err := ParseFilter("foo=(somevalue)")
			require.NoError(t, err)

			_, _, err = table.WhereClause(filter, "p_")
			assertions.ErrLike(
				t,
				err,
				[]any{"composite expressions in arguments not implemented yet"},
			)
		})

		t.Run("unsupported field LHS", func(t *testing.T) {
			filter, err := ParseFilter("foo.baz=blah")
			require.NoError(t, err)

			_, _, err = table.WhereClause(filter, "p_")
			assertions.ErrLike(t, err, []any{"fields are only supported for key value columns"})
		})

		t.Run("unsupported field RHS", func(t *testing.T) {
			filter, err := ParseFilter("foo=blah.baz")
			require.NoError(t, err)

			_, _, err = table.WhereClause(filter, "p_")
			assertions.ErrLike(t, err, []any{"fields not implemented yet"})
		})

		t.Run("field on RHS of has", func(t *testing.T) {
			filter, err := ParseFilter("foo:blah.baz")
			require.NoError(t, err)

			_, _, err = table.WhereClause(filter, "p_")
			assertions.ErrLike(
				t,
				err,
				[]any{"fields are not allowed on the RHS of has (:) operator"},
			)
		})

		t.Run("WithArgumentSubstitutor filter substituted", func(t *testing.T) {
			filter, err := ParseFilter("qux=somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "somevalue-v2",
				},
			}, pars)
			assert.Equal(t, "(db_qux = @p_0)", result)
		})

		t.Run("WithArgumentSubstitutor filter not supported", func(t *testing.T) {
			filter, err := ParseFilter("qux:some")
			require.NoError(t, err)

			_, _, err = table.WhereClause(filter, "p_")
			assertions.ErrLike(
				t,
				err,
				[]any{"cannot use has (:) operator on a field that have argSubstitute function"},
			)
		})

		t.Run("WithArgumentSubstitutor filter key value", func(t *testing.T) {
			filter, err := ParseFilter("quux.somekey=somevalue")
			require.NoError(t, err)

			result, pars, err := table.WhereClause(filter, "p_")
			require.NoError(t, err)
			assert.Equal(t, []QueryParameter{
				{
					Name:  "p_0",
					Value: "somekey",
				},
				{
					Name:  "p_1",
					Value: "somevalue-v2",
				},
			}, pars)
			assert.Equal(
				t,
				"(EXISTS (SELECT key, value FROM UNNEST(db_quux) WHERE key = @p_0 AND value = @p_1))",
				result,
			)
		})
	})

	t.Run("Complex filter", func(t *testing.T) {
		filter, err := ParseFilter(
			"implicit (foo=explicitone) OR -bar=explicittwo AND foo!=explicitthree OR baz:explicitfour",
		)
		require.NoError(t, err)

		result, pars, err := table.WhereClause(filter, "p_")
		require.NoError(t, err)
		assert.Equal(t, []QueryParameter{
			{
				Name:  "p_0",
				Value: "%implicit%",
			},
			{
				Name:  "p_1",
				Value: "explicitone",
			},
			{
				Name:  "p_2",
				Value: "explicittwo",
			},
			{
				Name:  "p_3",
				Value: "explicitthree",
			},
			{
				Name:  "p_4",
				Value: "%explicitfour%",
			},
		}, pars)
		assert.Equal(
			t,
			"((db_foo LIKE @p_0 OR db_bar LIKE @p_0) AND ((db_foo = @p_1) OR (NOT (db_bar = @p_2))) AND ((db_foo <> @p_3) OR (db_baz LIKE @p_4)))",
			result,
		)
	})
}

// ============================================================================
// Filter Generator Options Tests
// ============================================================================

func TestWhereClauseWithOptions(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("foo").
			WithDatabaseName("db_foo").
			Filterable().
			WithMatchModes(MatchModePrefix, MatchModeContains).
			Build(),
		NewColumn().WithFieldPath("bar").
			WithDatabaseName("db_bar").
			Filterable().
			WithMatchModes(MatchModeExact).
			Build(),
		NewColumn().WithFieldPath("body").
			WithDatabaseName("db_body").
			Filterable().
			WithMatchModes(MatchModeFullText).
			Build(),
	).Build()

	t.Run("prefix mode", func(t *testing.T) {
		filter, err := ParseFilter("foo:hello")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		require.NoError(t, err)
		assert.Equal(t, "(db_foo LIKE @p_0)", query)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "hello%"},
		}, params)
	})

	t.Run("exact mode", func(t *testing.T) {
		filter, err := ParseFilter("bar:hello")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		require.NoError(t, err)
		assert.Equal(t, "(db_bar = @p_0)", query)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "hello"},
		}, params)
	})

	t.Run("full text in postgres", func(t *testing.T) {
		filter, err := ParseFilter("body:alert")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectPostgres,
		})
		require.NoError(t, err)
		assert.Equal(
			t,
			"(to_tsvector('simple', db_body) @@ websearch_to_tsquery('simple', @p_0))",
			query,
		)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "alert"},
		}, params)
	})

	t.Run("full text in mysql", func(t *testing.T) {
		filter, err := ParseFilter("body:alert")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectMySQL,
		})
		require.NoError(t, err)
		assert.Equal(t, "(MATCH(db_body) AGAINST (@p_0 IN BOOLEAN MODE))", query)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "alert"},
		}, params)
	})

	t.Run("fallback to contains when dialect cannot do full text", func(t *testing.T) {
		filter, err := ParseFilter("body:alert")
		require.NoError(t, err)

		query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect: SQLDialectGeneric,
		})
		require.NoError(t, err)
		assert.Equal(t, "(db_body LIKE @p_0)", query)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "%alert%"},
		}, params)
	})

	t.Run("strict mode rejects unsupported full text dialect", func(t *testing.T) {
		filter, err := ParseFilter("body:alert")
		require.NoError(t, err)

		_, _, err = table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: true,
		})
		assertions.ErrLike(t, err, []any{`no supported match mode for field "body"`})
	})

	t.Run("strict mode rejects column with no match modes configured", func(t *testing.T) {
		tableNoModes := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				Build(),
		).Build()

		filter, err := ParseFilter("name:test")
		require.NoError(t, err)

		_, _, err = tableNoModes.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: true,
		})
		assertions.ErrLike(t, err, []any{`no match mode configured for field "name"`})
	})

	t.Run("non-strict mode uses fallback for column with no match modes", func(t *testing.T) {
		tableNoModes := NewTable().WithColumns(
			NewColumn().WithFieldPath("name").
				WithDatabaseName("db_name").
				Filterable().
				Build(),
		).Build()

		filter, err := ParseFilter("name:test")
		require.NoError(t, err)

		query, params, err := tableNoModes.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
			Dialect:    SQLDialectGeneric,
			StrictMode: false,
		})
		require.NoError(t, err)
		assert.Equal(t, "(db_name LIKE @p_0)", query)
		assert.Equal(t, []QueryParameter{
			{Name: "p_0", Value: "%test%"},
		}, params)
	})
}

func TestWhereClauseOrEqualityUsesIN(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("foo").WithDatabaseName("db_foo").Filterable().Build(),
	).Build()
	filter, err := ParseFilter("foo=one OR foo=two OR foo=three")
	require.NoError(t, err)

	query, params, err := table.WhereClause(filter, "p_")
	require.NoError(t, err)
	assert.Equal(t, "(db_foo IN (@p_0, @p_1, @p_2))", query)
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "one"},
		{Name: "p_1", Value: "two"},
		{Name: "p_2", Value: "three"},
	}, params)
}

func TestWhereClauseWithOptionsCompositeIndexRangeReordering(t *testing.T) {
	table := NewTable().WithColumns(
		NewColumn().WithFieldPath("status").WithDatabaseName("status").Filterable().Build(),
		NewColumn().WithFieldPath("user_id").WithDatabaseName("user_id").Filterable().Build(),
		NewColumn().WithFieldPath("created_at").WithDatabaseName("created_at").Filterable().Build(),
	).Build()
	table.CompositeIndexes = []CompositeIndex{
		{
			Name:    "idx_status_user_created",
			Columns: []string{"status", "user_id", "created_at"},
		},
	}

	filter, err := ParseFilter(`created_at>"2024-01-01" AND user_id=123 AND status="active"`)
	require.NoError(t, err)

	query, params, err := table.WhereClauseWithOptions(filter, "p_", WhereClauseOptions{
		EnableCompositeIndexOptimization: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "((status = @p_0) AND (user_id = @p_1) AND (created_at > @p_2))", query)
	assert.Equal(t, []QueryParameter{
		{Name: "p_0", Value: "active"},
		{Name: "p_1", Value: "123"},
		{Name: "p_2", Value: "2024-01-01"},
	}, params)
}

// ============================================================================
// Benchmarks
// ============================================================================

// Helper function for benchmarks
func setupTestTable() *Table {
	return NewTable().
		WithColumns(
			NewColumn().WithFieldPath("foo").WithDatabaseName("foo").Filterable().Build(),
			NewColumn().WithFieldPath("bar").WithDatabaseName("bar").Filterable().Build(),
			NewColumn().WithFieldPath("baz").WithDatabaseName("baz").Filterable().Build(),
			NewColumn().WithFieldPath("qux").WithDatabaseName("qux").Filterable().Build(),
			NewColumn().WithFieldPath("test").WithDatabaseName("test").Filterable().Build(),
			NewColumn().WithFieldPath("status").WithDatabaseName("status").Filterable().Build(),
			NewColumn().WithFieldPath("priority").WithDatabaseName("priority").Filterable().Build(),
			NewColumn().WithFieldPath("deleted").
				WithDatabaseName("deleted").
				Filterable().
				Bool().
				Build(),
			NewColumn().WithFieldPath("created_at").
				WithDatabaseName("created_at").
				Filterable().
				Build(),
		).
		Build()
}

// Filter Parser Benchmarks

func BenchmarkParseFilter_Simple(b *testing.B) {
	filter := "foo=bar"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_Medium(b *testing.B) {
	filter := "foo=bar AND baz=qux"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_Complex(b *testing.B) {
	filter := "foo=bar AND baz=qux OR NOT test=value"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_VeryComplex(b *testing.B) {
	filter := "status=\"ACTIVE\" AND (priority=HIGH OR priority=URGENT) AND NOT deleted=true AND created_at > \"2024-01-01\""
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_WithGlobal(b *testing.B) {
	filter := "searchterm"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_WithKeyValue(b *testing.B) {
	filter := "labels.key=value"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_HasOperator(b *testing.B) {
	filter := "description:keyword"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_MultipleClauses(b *testing.B) {
	filter := "a=1 AND b=2 AND c=3 AND d=4 AND e=5 AND f=6 AND g=7 AND h=8 AND i=9 AND j=10"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_OrChain(b *testing.B) {
	filter := "a=1 OR b=2 OR c=3 OR d=4 OR e=5 OR f=6 OR g=7 OR h=8 OR i=9 OR j=10"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseFilter_Nested(b *testing.B) {
	filter := "((a=1 OR b=2) AND (c=3 OR d=4)) OR ((e=5 OR f=6) AND (g=7 OR h=8))"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := ParseFilter(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Filter Generator Benchmarks

func BenchmarkWhereClause_Simple(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("foo=bar")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_Medium(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("foo=bar AND baz=qux")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_Complex(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("foo=bar AND baz=qux OR NOT test=value")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_VeryComplex(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter(
		"status=\"ACTIVE\" AND (priority=HIGH OR priority=URGENT) AND NOT deleted=true",
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_HasOperator(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("foo:keyword")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_MultipleClauses(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter(
		"foo=bar AND baz=qux AND test=value AND status=ACTIVE AND priority=HIGH",
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_OrChain(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("foo=bar OR baz=qux OR test=value OR status=ACTIVE OR priority=HIGH")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_Nested(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("(foo=bar OR baz=qux) AND (test=value OR status=ACTIVE)")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_WithNegation(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("NOT deleted=true AND status=ACTIVE")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereClause_NotEqual(b *testing.B) {
	table := setupTestTable()
	filter, err := ParseFilter("status!=DELETED AND priority!=LOW")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := table.WhereClause(filter, "p_")
		if err != nil {
			b.Fatal(err)
		}
	}
}
