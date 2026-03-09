package googleaip

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMustCompilePattern(t *testing.T) {
	pattern := MustCompilePattern("publishers/{publisher}/books/{book}")

	require.Equal(t, "publishers/{publisher}/books/{book}", pattern.Pattern)
	require.Equal(t, []string{"publisher", "book"}, pattern.Variables())
}

func TestMustCompilePatternPanicsOnInvalidPattern(t *testing.T) {
	require.Panics(t, func() {
		MustCompilePattern("publishers/{publisher/books/{book}")
	})
}

func TestResourceDescriptorParse(t *testing.T) {
	descriptor := ResourceDescriptor{
		Type: "library.googleapis.com/Book",
		Patterns: []ResourcePattern{
			MustCompilePattern("publishers/{publisher}/books/{book}"),
			MustCompilePattern("archives/{archive}/books/{book}"),
		},
	}

	match, err := descriptor.Parse("publishers/p1/books/b1")
	require.NoError(t, err)
	require.Equal(t, "library.googleapis.com/Book", match.DescriptorType)
	require.Equal(t, "publishers/{publisher}/books/{book}", match.Pattern)
	require.Equal(t, map[string]string{
		"publisher": "p1",
		"book":      "b1",
	}, match.Values)
}

func TestResourceDescriptorValidate(t *testing.T) {
	descriptor := ResourceDescriptor{
		Type: "library.googleapis.com/Book",
		Patterns: []ResourcePattern{
			MustCompilePattern("publishers/{publisher}/books/{book}"),
		},
	}

	require.NoError(t, descriptor.Validate("publishers/p1/books/b1"))
	require.Error(t, descriptor.Validate("archives/a1/books/b1"))
}

func TestResourceDescriptorFormatWithPattern(t *testing.T) {
	descriptor := ResourceDescriptor{
		Type: "library.googleapis.com/Book",
		Patterns: []ResourcePattern{
			MustCompilePattern("publishers/{publisher}/books/{book}"),
		},
	}

	formatted, err := descriptor.FormatWithPattern(
		"publishers/{publisher}/books/{book}",
		map[string]string{
			"publisher": "p1",
			"book":      "b1",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "publishers/p1/books/b1", formatted)
}

func TestResourceDescriptorFormatWithPatternErrors(t *testing.T) {
	descriptor := ResourceDescriptor{
		Type: "library.googleapis.com/Book",
		Patterns: []ResourcePattern{
			MustCompilePattern("publishers/{publisher}/books/{book}"),
		},
	}

	_, err := descriptor.FormatWithPattern(
		"publishers/{publisher}/books/{book}",
		map[string]string{"publisher": "p1"},
	)
	require.Error(t, err)

	_, err = descriptor.FormatWithPattern(
		"publishers/{publisher}/books/{book}",
		map[string]string{
			"publisher": "p1",
			"book":      "books/b1",
		},
	)
	require.Error(t, err)

	_, err = descriptor.FormatWithPattern(
		"archives/{archive}/books/{book}",
		map[string]string{
			"archive": "a1",
			"book":    "b1",
		},
	)
	require.Error(t, err)
}

func TestResourceDescriptorClone(t *testing.T) {
	descriptor := ResourceDescriptor{
		Type: "library.googleapis.com/Book",
		Patterns: []ResourcePattern{
			MustCompilePattern("publishers/{publisher}/books/{book}"),
		},
	}

	clone := descriptor.Clone()
	clone.Patterns[0] = MustCompilePattern("archives/{archive}/books/{book}")

	require.Equal(t, "publishers/{publisher}/books/{book}", descriptor.Patterns[0].Pattern)
}
