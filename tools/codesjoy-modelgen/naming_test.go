package main

import "testing"

func TestToPascalCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "snake", input: "user_id", want: "UserID"},
		{name: "acronym", input: "request_url", want: "RequestURL"},
		{name: "numeric prefix", input: "1st_order", want: "X1stOrder"},
		{name: "mixed symbols", input: "http-status", want: "HTTPStatus"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ToPascalCase(tc.input)
			if got != tc.want {
				t.Fatalf("ToPascalCase(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDedupeFieldNames(t *testing.T) {
	t.Parallel()

	cols := []ResolvedColumn{
		{Name: "user_id", GoField: "UserID"},
		{Name: "user-id", GoField: "UserID"},
		{Name: "user_id_2", GoField: "UserID"},
	}
	got, err := dedupeFieldNames(cols)
	if err != nil {
		t.Fatalf("dedupeFieldNames() error = %v", err)
	}
	if got[0].GoField != "UserID" {
		t.Fatalf("first field mismatch: %q", got[0].GoField)
	}
	if got[1].GoField != "UserID2" {
		t.Fatalf("second field mismatch: %q", got[1].GoField)
	}
	if got[2].GoField != "UserID3" {
		t.Fatalf("third field mismatch: %q", got[2].GoField)
	}
}
