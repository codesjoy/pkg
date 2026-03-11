package main

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	annotationspb "google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestParseFeatureSet(t *testing.T) {
	features, err := parseFeatureSet("resources")
	require.NoError(t, err)
	require.True(t, features.resources)

	_, err = parseFeatureSet("field-behavior")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown feature "field-behavior"`)

	_, err = parseFeatureSet("method_signature")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown feature "method_signature"`)

	_, err = parseFeatureSet("unknown")
	require.Error(t, err)
}

func TestGenerateFilesCreatesPerProtoOutputs(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"publisher.proto", "book.proto"},
		[]*descriptorpb.FileDescriptorProto{
			newPublisherFile(),
			newBookFile(),
		},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	files := generatedFilesBySuffix(gen, "_google_aip.pb.go")
	require.Len(t, files, 2)

	publisherContent := generatedFileContent(gen, "publisher_google_aip.pb.go")
	require.NotEmpty(t, publisherContent)
	assert.Contains(
		t,
		publisherContent,
		"// PublisherNamePattern is a supported resource name pattern for Publisher.",
	)
	assert.Contains(t, publisherContent, `const PublisherNamePattern = "publishers/{publisher}"`)
	assert.Contains(
		t,
		publisherContent,
		"// ParsedPublisherName contains the typed components of a parsed Publisher resource name.",
	)
	assert.Contains(t, publisherContent, "type ParsedPublisherName struct {")
	assert.Regexp(t, `Publisher\s+string`, publisherContent)
	assert.Contains(
		t,
		publisherContent,
		"// ParsePublisherName parses a Publisher resource name into typed fields.",
	)
	assert.Contains(
		t,
		publisherContent,
		"func ParsePublisherName(name string) (ParsedPublisherName, error)",
	)
	assert.Contains(
		t,
		publisherContent,
		"// ValidatePublisherName reports whether name is a valid Publisher resource name.",
	)
	assert.Contains(t, publisherContent, "func ValidatePublisherName(name string) error")
	assert.Contains(
		t,
		publisherContent,
		"// ParseName parses the resource name stored on Publisher.",
	)
	assert.Contains(
		t,
		publisherContent,
		"func (x *Publisher) ParseName() (ParsedPublisherName, error)",
	)
	assert.Contains(
		t,
		publisherContent,
		"// ValidateName reports whether the resource name stored on Publisher is valid.",
	)
	assert.Contains(t, publisherContent, "func (x *Publisher) ValidateName() error")
	assert.Contains(t, publisherContent, `parts := strings.Split(name, "/")`)
	assert.Contains(t, publisherContent, "return ParsePublisherName(x.Name)")
	assert.Contains(t, publisherContent, "return ValidatePublisherName(x.Name)")
	assert.Regexp(t, `Publisher:\s+parts\[1\],`, publisherContent)
	assert.Contains(
		t,
		publisherContent,
		"// PublisherNameParts contains the typed components used to format a Publisher resource name.",
	)
	assert.Contains(t, publisherContent, "type PublisherNameParts struct {")
	assert.Contains(
		t,
		publisherContent,
		"// FormatPublisherNameWithPattern formats a supported resource name pattern for Publisher.",
	)
	assert.Contains(
		t,
		publisherContent,
		"func FormatPublisherNameWithPattern(pattern string, parts PublisherNameParts) (string, error)",
	)
	assert.Contains(
		t,
		publisherContent,
		"// FormatPublisherName formats the only supported resource name pattern for Publisher.",
	)
	assert.Contains(
		t,
		publisherContent,
		"func FormatPublisherName(parts PublisherNameParts) (string, error)",
	)
	assert.Contains(
		t,
		publisherContent,
		"return FormatPublisherNameWithPattern(PublisherNamePattern, parts)",
	)
	assert.Contains(
		t,
		publisherContent,
		"// FillNameFromParts formats the only supported resource name pattern and writes it back to Name.",
	)
	assert.Contains(
		t,
		publisherContent,
		"func (x *Publisher) FillNameFromParts(parts PublisherNameParts) error",
	)
	assert.Contains(
		t,
		publisherContent,
		"return x.FillNameWithPatternFromParts(PublisherNamePattern, parts)",
	)
	assert.Contains(
		t,
		publisherContent,
		"// FillNameWithPatternFromParts formats a supported resource name pattern and writes it back to Name.",
	)
	assert.Contains(
		t,
		publisherContent,
		"func (x *Publisher) FillNameWithPatternFromParts(pattern string, parts PublisherNameParts) error",
	)
	assert.Contains(
		t,
		publisherContent,
		"// FillName formats the only supported resource name pattern and writes it back to Name.",
	)
	assert.Contains(
		t,
		publisherContent,
		"func (x *Publisher) FillName(values map[string]string) error",
	)
	assert.Contains(
		t,
		publisherContent,
		"// Deprecated: Use FillNameFromParts instead.",
	)
	assert.Contains(t, publisherContent, "formatted, err := formatPublisherNameFromMap(values)")
	assert.Contains(t, publisherContent, "func formatPublisherNameFromMap(values map[string]string) (string, error)")
	assert.Contains(
		t,
		publisherContent,
		"func formatPublisherNameWithPatternFromMap(pattern string, values map[string]string) (string, error)",
	)
	assert.Contains(
		t,
		publisherContent,
		"// Deprecated: Use FillNameWithPatternFromParts instead.",
	)
	assert.Contains(t, publisherContent, "switch pattern {")
	assert.NotContains(t, publisherContent, "googleaip.")
	assert.NotContains(t, publisherContent, "func (x *Publisher) ParseParent(parent string)")
	assert.NotContains(t, publisherContent, "func (x *Publisher) ValidateParent(parent string)")
	assert.NotContains(
		t,
		publisherContent,
		"func (x *Book) FillNameWithPattern(pattern string, values map[string]string) error",
	)
	assert.NotContains(t, publisherContent, "GoogleAIPResourceReference(")
	assert.NotContains(t, publisherContent, "runtime/googleaip")

	bookContent := generatedFileContent(gen, "book_google_aip.pb.go")
	require.NotEmpty(t, bookContent)
	assert.Contains(
		t,
		bookContent,
		"// BookNamePattern1 is a supported resource name pattern for Book.",
	)
	assert.Contains(t, bookContent, `BookNamePattern1 = "publishers/{publisher}/books/{book}"`)
	assert.Contains(
		t,
		bookContent,
		"// BookNamePattern2 is a supported resource name pattern for Book.",
	)
	assert.Contains(t, bookContent, `BookNamePattern2 = "archives/{archive}/books/{book}"`)
	assert.Contains(
		t,
		bookContent,
		"// ParsedBookName contains the typed components of a parsed Book resource name.",
	)
	assert.Contains(t, bookContent, "type ParsedBookName struct {")
	assert.Regexp(t, `DescriptorType\s+string`, bookContent)
	assert.Regexp(t, `Pattern\s+string`, bookContent)
	assert.Regexp(t, `Publisher\s+string`, bookContent)
	assert.Regexp(t, `Book\s+string`, bookContent)
	assert.Regexp(t, `Archive\s+string`, bookContent)
	assert.Contains(
		t,
		bookContent,
		"// ParseBookName parses a Book resource name into typed fields.",
	)
	assert.Contains(t, bookContent, "func ParseBookName(name string) (ParsedBookName, error)")
	assert.Contains(
		t,
		bookContent,
		"// ValidateBookName reports whether name is a valid Book resource name.",
	)
	assert.Contains(t, bookContent, "func ValidateBookName(name string) error")
	assert.Contains(t, bookContent, `parts := strings.Split(name, "/")`)
	assert.Contains(
		t,
		bookContent,
		"// BookNameParts contains the typed components used to format a Book resource name.",
	)
	assert.Contains(t, bookContent, "type BookNameParts struct {")
	assert.Contains(
		t,
		bookContent,
		"// FormatBookNameWithPattern formats a supported resource name pattern for Book.",
	)
	assert.Contains(
		t,
		bookContent,
		"func FormatBookNameWithPattern(pattern string, parts BookNameParts) (string, error)",
	)
	assert.Contains(
		t,
		bookContent,
		"// FillNameWithPatternFromParts formats a supported resource name pattern and writes it back to Name.",
	)
	assert.Contains(
		t,
		bookContent,
		"func (x *Book) FillNameWithPatternFromParts(pattern string, parts BookNameParts) error",
	)
	assert.Contains(
		t,
		bookContent,
		"// FillNameWithPattern formats a supported resource name pattern and writes it back to Name.",
	)
	assert.Contains(
		t,
		bookContent,
		"func (x *Book) FillNameWithPattern(pattern string, values map[string]string) error",
	)
	assert.Contains(
		t,
		bookContent,
		"func formatBookNameWithPatternFromMap(pattern string, values map[string]string) (string, error)",
	)
	assert.Contains(
		t,
		bookContent,
		"// Deprecated: Use FillNameWithPatternFromParts instead.",
	)
	assert.Contains(t, bookContent, "return ParseBookName(x.Name)")
	assert.Contains(t, bookContent, "return ValidateBookName(x.Name)")
	assert.Regexp(t, `Publisher:\s+parts\[1\],`, bookContent)
	assert.Regexp(t, `Book:\s+parts\[3\],`, bookContent)
	assert.Regexp(t, `Archive:\s+parts\[1\],`, bookContent)
	assert.Contains(
		t,
		bookContent,
		"// ParsedBookParent contains the typed components of a parsed parent for Book.",
	)
	assert.Contains(t, bookContent, "type ParsedBookParent struct {")
	assert.Contains(
		t,
		bookContent,
		"// ParseBookParent parses a parent resource name accepted by Book into typed fields.",
	)
	assert.Contains(t, bookContent, "func ParseBookParent(parent string) (ParsedBookParent, error)")
	assert.Contains(
		t,
		bookContent,
		"// ValidateBookParent reports whether parent is a valid parent resource name for Book.",
	)
	assert.Contains(t, bookContent, "func ValidateBookParent(parent string) error")
	assert.Contains(t, bookContent, "switch pattern {")
	assert.Contains(
		t,
		bookContent,
		"// ParseParent parses a parent resource name accepted by Book.",
	)
	assert.Contains(
		t,
		bookContent,
		"func (x *Book) ParseParent(parent string) (ParsedBookParent, error)",
	)
	assert.Contains(
		t,
		bookContent,
		"// ValidateParent reports whether parent is a valid parent resource name for Book.",
	)
	assert.Contains(t, bookContent, "func (x *Book) ValidateParent(parent string) error")
	assert.Contains(t, bookContent, `parts := strings.Split(parent, "/")`)
	assert.Regexp(t, `Publisher:\s+parts\[1\],`, bookContent)
	assert.Regexp(t, `Archive:\s+parts\[1\],`, bookContent)
	assert.Contains(t, bookContent, "return ParseBookParent(parent)")
	assert.Contains(t, bookContent, "return ValidateBookParent(parent)")
	assert.NotContains(t, bookContent, "func (x *Book) FillName(values map[string]string) error")
	assert.NotContains(t, bookContent, "func FormatBookName(parts BookNameParts)")
	assert.NotContains(t, bookContent, "googleaip.")
	assert.Contains(t, bookContent, "nil *Book receiver")
	assert.NotContains(t, bookContent, "func (x *ListBooksRequest) ParseParent()")
	assert.NotContains(t, bookContent, "GoogleAIPResourceReference(")
	assert.Contains(t, bookContent, "library.googleapis.com/Archive")
	assert.NotContains(t, bookContent, "runtime/googleaip")
}

func TestGenerateFilesHonorsFeatureFiltering(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"publisher.proto"},
		[]*descriptorpb.FileDescriptorProto{newPublisherFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	content := generatedFileContent(gen, "publisher_google_aip.pb.go")
	require.NotEmpty(t, content)
	assert.Contains(t, content, `const PublisherNamePattern = "publishers/{publisher}"`)
	assert.Contains(t, content, "func ParsePublisherName(name string) (ParsedPublisherName, error)")
	assert.Contains(t, content, "func ValidatePublisherName(name string) error")
	assert.Contains(t, content, "func (x *Publisher) ParseName() (ParsedPublisherName, error)")
	assert.NotContains(t, content, "GoogleAIPResourceReference(")
	assert.NotContains(t, content, "func GoogleAIPLookupResource(resourceType string)")
}

func TestGenerateFilesSkipsWhenNoAnnotations(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"plain.proto"},
		[]*descriptorpb.FileDescriptorProto{newPlainFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))
	assert.Empty(t, generatedFilesBySuffix(gen, "_google_aip.pb.go"))
}

func TestGenerateFilesSkipsWhenOnlyFileResourceDefinitionsExist(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"definitions.proto"},
		[]*descriptorpb.FileDescriptorProto{newFileDefinitionsOnlyFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))
	assert.Empty(t, generatedFilesBySuffix(gen, "_google_aip.pb.go"))
}

func TestGenerateFilesSkipsWhenOnlyResourceReferencesExist(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"reference_only.proto"},
		[]*descriptorpb.FileDescriptorProto{newResourceReferenceOnlyFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))
	assert.Empty(t, generatedFilesBySuffix(gen, "_google_aip.pb.go"))
}

func TestGenerateFilesUsesConfiguredNameField(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"shelf.proto"},
		[]*descriptorpb.FileDescriptorProto{newNamedResourceFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	content := generatedFileContent(gen, "shelf_google_aip.pb.go")
	require.NotEmpty(t, content)
	assert.Contains(t, content, "type ParsedShelfName struct {")
	assert.Regexp(t, `Shelf\s+string`, content)
	assert.Contains(t, content, "func ParseShelfName(name string) (ParsedShelfName, error)")
	assert.Contains(t, content, "func ValidateShelfName(name string) error")
	assert.Contains(t, content, "return ValidateShelfName(x.ResourceName)")
	assert.Contains(t, content, "return ParseShelfName(x.ResourceName)")
	assert.Regexp(t, `Shelf:\s+parts\[1\],`, content)
	assert.Contains(t, content, "x.ResourceName = formatted")
	assert.Contains(t, content, `const ShelfNamePattern = "shelves/{shelf}"`)
	assert.Contains(t, content, "type ShelfNameParts struct {")
	assert.Contains(t, content, "func FormatShelfName(parts ShelfNameParts) (string, error)")
	assert.Contains(t, content, "func (x *Shelf) FillNameFromParts(parts ShelfNameParts) error")
	assert.Contains(t, content, "return x.FillNameWithPatternFromParts(ShelfNamePattern, parts)")
	assert.NotContains(t, content, "runtime/googleaip")
}

func TestGenerateFilesErrorsOnCrossFileDuplicateResourceTypes(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"publisher.proto", "duplicate.proto"},
		[]*descriptorpb.FileDescriptorProto{
			newPublisherFile(),
			newDuplicatePublisherFile(),
		},
	)
	require.NoError(t, err)

	err = generateFiles(gen, featureSet{resources: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate resource type "library.googleapis.com/Publisher"`)
}

func TestGenerateFilesErrorsWhenNameFieldIsMissing(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"broken.proto"},
		[]*descriptorpb.FileDescriptorProto{newMissingNameFieldFile()},
	)
	require.NoError(t, err)

	err = generateFiles(gen, featureSet{resources: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `resource name_field "resource_name" not found`)
}

func TestGenerateFilesErrorsWhenNameFieldIsNotString(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"broken.proto"},
		[]*descriptorpb.FileDescriptorProto{newNonStringNameFieldFile()},
	)
	require.NoError(t, err)

	err = generateFiles(gen, featureSet{resources: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `resource name_field "resource_id"`)
	assert.Contains(t, err.Error(), "must target a singular string field")
}

func TestGenerateFilesGeneratesResourceParentHelpersWithoutRequestField(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"publisher.proto", "book.proto"},
		[]*descriptorpb.FileDescriptorProto{
			newPublisherFile(),
			newBookFile(),
		},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	content := generatedFileContent(gen, "book_google_aip.pb.go")
	require.NotEmpty(t, content)
	assert.Contains(t, content, `"library.googleapis.com/Publisher"`)
	assert.Contains(t, content, `"library.googleapis.com/Archive"`)
	assert.Contains(t, content, "func ParseBookParent(parent string) (ParsedBookParent, error)")
	assert.Contains(t, content, "func ValidateBookParent(parent string) error")
	assert.Contains(
		t,
		content,
		"func (x *Book) ParseParent(parent string) (ParsedBookParent, error)",
	)
	assert.Contains(t, content, "func (x *Book) ValidateParent(parent string) error")
	assert.Contains(t, content, "nil *Book receiver")
	assert.Regexp(t, `Publisher:\s+parts\[1\],`, content)
	assert.Regexp(t, `Archive:\s+parts\[1\],`, content)
	assert.Contains(t, content, "return ParseBookParent(parent)")
	assert.Contains(t, content, "return ValidateBookParent(parent)")
	assert.NotContains(t, content, "func (x *Book) FillParent(")
	assert.NotContains(t, content, "func (x *Book) ParentID()")
}

func TestGenerateFilesGeneratesResourceParentHelpersForExampleLayout(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"publisher.proto", "book.proto"},
		[]*descriptorpb.FileDescriptorProto{
			newExamplePublisherOnlyFile(),
			newExampleBookLayoutFile(),
		},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	content := generatedFileContent(gen, "book_google_aip.pb.go")
	require.NotEmpty(t, content)
	assert.Contains(t, content, "func ParseBookParent(parent string) (ParsedBookParent, error)")
	assert.Contains(
		t,
		content,
		"func (x *Book) ParseParent(parent string) (ParsedBookParent, error)",
	)
	assert.Contains(t, content, "func (x *Book) ValidateParent(parent string) error")
	assert.NotContains(t, content, "func (x *ListBooksRequest) ParseParent()")
}

func TestGenerateFilesGeneratesResourceParentHelpersWhenParentDescriptorIsMissing(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"orphan_book.proto"},
		[]*descriptorpb.FileDescriptorProto{newOrphanBookFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	content := generatedFileContent(gen, "orphan_book_google_aip.pb.go")
	require.NotEmpty(t, content)
	assert.Regexp(t, `DescriptorType:\s+""`, content)
	assert.Regexp(t, `Pattern:\s+"publishers/\{publisher\}"`, content)
	assert.Regexp(t, `Publisher:\s+parts\[1\],`, content)
	assert.Contains(t, content, "func ParseBookParent(parent string) (ParsedBookParent, error)")
	assert.Contains(
		t,
		content,
		"func (x *Book) ParseParent(parent string) (ParsedBookParent, error)",
	)
	assert.Contains(t, content, "func (x *Book) ValidateParent(parent string) error")
}

func TestGenerateFilesGeneratesMixedResolvedAndUnresolvedResourceParents(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"publisher.proto", "mixed_parent_book.proto"},
		[]*descriptorpb.FileDescriptorProto{
			newPublisherFile(),
			newMixedParentBookFile(),
		},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	content := generatedFileContent(gen, "mixed_parent_book_google_aip.pb.go")
	require.NotEmpty(t, content)
	assert.Contains(t, content, `"library.googleapis.com/Publisher"`)
	assert.Regexp(t, `Pattern:\s+"publishers/\{publisher\}"`, content)
	assert.Regexp(t, `DescriptorType:\s+""`, content)
	assert.Regexp(t, `Pattern:\s+"folders/\{folder\}"`, content)
	assert.Contains(t, content, "func ParseBookParent(parent string) (ParsedBookParent, error)")
	assert.Contains(
		t,
		content,
		"func (x *Book) ParseParent(parent string) (ParsedBookParent, error)",
	)
	assert.Contains(t, content, "func (x *Book) ValidateParent(parent string) error")
}

func TestGenerateFilesSkipsResourceParentHelpersWhenChildPatternCannotDeriveParent(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"broken_parent_pattern.proto"},
		[]*descriptorpb.FileDescriptorProto{newBrokenParentPatternFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	content := generatedFileContent(gen, "broken_parent_pattern_google_aip.pb.go")
	require.NotEmpty(t, content)
	assert.NotContains(t, content, "func (x *WeirdChild) ParseParent(parent string)")
	assert.NotContains(t, content, "func (x *WeirdChild) ValidateParent(parent string)")
}

func newTestPluginWithFiles(
	filesToGenerate []string,
	files []*descriptorpb.FileDescriptorProto,
) (*protogen.Plugin, error) {
	return protogen.Options{}.New(&pluginpb.CodeGeneratorRequest{
		FileToGenerate: filesToGenerate,
		ProtoFile:      files,
	})
}

func generatedFileContent(gen *protogen.Plugin, suffix string) string {
	files := generatedFilesBySuffix(gen, suffix)
	if len(files) == 0 {
		return ""
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return files[names[0]]
}

func generatedFilesBySuffix(gen *protogen.Plugin, suffix string) map[string]string {
	out := make(map[string]string)
	for _, file := range gen.Response().File {
		if strings.HasSuffix(file.GetName(), suffix) {
			out[file.GetName()] = file.GetContent()
		}
	}
	return out
}

func newPublisherFile() *descriptorpb.FileDescriptorProto {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("publisher.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage(
				"Publisher",
				"library.googleapis.com/Publisher",
				[]string{"publishers/{publisher}"},
			),
			newGetBookRequestMessage(),
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("LibraryService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("GetBook"),
						InputType:  proto.String(".codesjoy.example.library.v1.GetBookRequest"),
						OutputType: proto.String(".codesjoy.example.library.v1.Publisher"),
						Options:    &descriptorpb.MethodOptions{},
					},
				},
			},
		},
	}
	return file
}

func newBookFile() *descriptorpb.FileDescriptorProto {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("book.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage(
				"Book",
				"library.googleapis.com/Book",
				[]string{
					"publishers/{publisher}/books/{book}",
					"archives/{archive}/books/{book}",
				},
			),
		},
	}

	proto.SetExtension(
		file.Options,
		annotationspb.E_ResourceDefinition,
		[]*annotationspb.ResourceDescriptor{
			{
				Type:     "library.googleapis.com/Archive",
				Pattern:  []string{"archives/{archive}"},
				Plural:   "archives",
				Singular: "archive",
			},
		},
	)
	return file
}

func newOrphanBookFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("orphan_book.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage(
				"Book",
				"library.googleapis.com/Book",
				[]string{"publishers/{publisher}/books/{book}"},
			),
		},
	}
}

func newMixedParentBookFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("mixed_parent_book.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage(
				"Book",
				"library.googleapis.com/Book",
				[]string{
					"publishers/{publisher}/books/{book}",
					"folders/{folder}/books/{book}",
				},
			),
		},
	}
}

func newFileDefinitionsOnlyFile() *descriptorpb.FileDescriptorProto {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("definitions.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
	}

	proto.SetExtension(
		file.Options,
		annotationspb.E_ResourceDefinition,
		[]*annotationspb.ResourceDescriptor{
			{
				Type:     "library.googleapis.com/Archive",
				Pattern:  []string{"archives/{archive}"},
				Plural:   "archives",
				Singular: "archive",
			},
		},
	)
	return file
}

func newPlainFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("plain.proto"),
		Package: proto.String("codesjoy.example.plain.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/plain/v1;plainv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("PlainMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
				Options: &descriptorpb.MessageOptions{},
			},
		},
	}
}

func newResourceMessage(
	name, resourceType string,
	patterns []string,
) *descriptorpb.DescriptorProto {
	return newResourceMessageWithNameField(
		name,
		resourceType,
		patterns,
		"name",
		stringField("name", 1),
	)
}

func newResourceMessageWithNameField(
	name string,
	resourceType string,
	patterns []string,
	nameField string,
	fields ...*descriptorpb.FieldDescriptorProto,
) *descriptorpb.DescriptorProto {
	message := &descriptorpb.DescriptorProto{
		Name:    proto.String(name),
		Field:   fields,
		Options: &descriptorpb.MessageOptions{},
	}

	proto.SetExtension(message.Options, annotationspb.E_Resource, &annotationspb.ResourceDescriptor{
		Type:      resourceType,
		Pattern:   patterns,
		Plural:    strings.ToLower(name) + "s",
		Singular:  strings.ToLower(name),
		NameField: nameField,
	})
	return message
}

func newNamedResourceFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("shelf.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessageWithNameField(
				"Shelf",
				"library.googleapis.com/Shelf",
				[]string{"shelves/{shelf}"},
				"resource_name",
				stringField("resource_name", 1),
			),
		},
	}
}

func newMissingNameFieldFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("broken.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessageWithNameField(
				"BrokenResource",
				"library.googleapis.com/BrokenResource",
				[]string{"broken/{resource}"},
				"resource_name",
				stringField("name", 1),
			),
		},
	}
}

func newNonStringNameFieldFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("broken.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessageWithNameField(
				"BrokenNumericResource",
				"library.googleapis.com/BrokenNumericResource",
				[]string{"broken/{resource}"},
				"resource_id",
				int32Field("resource_id", 1),
			),
		},
	}
}

func newDuplicatePublisherFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("duplicate.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage(
				"DuplicatePublisher",
				"library.googleapis.com/Publisher",
				[]string{"duplicate_publishers/{publisher}"},
			),
		},
	}
}

func newGetBookRequestMessage() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("GetBookRequest"),
		Field: []*descriptorpb.FieldDescriptorProto{
			stringField("name", 1),
			stringField("view", 2),
		},
		Options: &descriptorpb.MessageOptions{},
	}
}

func newListBooksFile() *descriptorpb.FileDescriptorProto {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("list_books.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage(
				"Book",
				"library.googleapis.com/Book",
				[]string{
					"publishers/{publisher}/books/{book}",
					"archives/{archive}/books/{book}",
				},
			),
			newSimpleMessage("ListBooksRequest", stringField("parent", 1)),
		},
	}

	proto.SetExtension(
		file.Options,
		annotationspb.E_ResourceDefinition,
		[]*annotationspb.ResourceDescriptor{
			{
				Type:     "library.googleapis.com/Archive",
				Pattern:  []string{"archives/{archive}"},
				Plural:   "archives",
				Singular: "archive",
			},
		},
	)
	return file
}

func newExamplePublisherOnlyFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("publisher.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage(
				"Publisher",
				"library.googleapis.com/Publisher",
				[]string{"publishers/{publisher}"},
			),
		},
	}
}

func newExampleBookLayoutFile() *descriptorpb.FileDescriptorProto {
	file := newListBooksFile()
	file.Name = proto.String("book.proto")
	file.MessageType = []*descriptorpb.DescriptorProto{
		newResourceMessage(
			"Book",
			"library.googleapis.com/Book",
			[]string{
				"publishers/{publisher}/books/{book}",
				"archives/{archive}/books/{book}",
			},
		),
		newGetBookRequestMessage(),
		newListBooksRequestMessage(),
		{
			Name: proto.String("ListBooksResponse"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name:     proto.String("books"),
					Number:   proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".codesjoy.example.library.v1.Book"),
				},
			},
			Options: &descriptorpb.MessageOptions{},
		},
	}
	file.Service = []*descriptorpb.ServiceDescriptorProto{
		{
			Name: proto.String("LibraryService"),
			Method: []*descriptorpb.MethodDescriptorProto{
				{
					Name:       proto.String("GetBook"),
					InputType:  proto.String(".codesjoy.example.library.v1.GetBookRequest"),
					OutputType: proto.String(".codesjoy.example.library.v1.Book"),
					Options:    &descriptorpb.MethodOptions{},
				},
				{
					Name:       proto.String("ListBooks"),
					InputType:  proto.String(".codesjoy.example.library.v1.ListBooksRequest"),
					OutputType: proto.String(".codesjoy.example.library.v1.ListBooksResponse"),
					Options:    &descriptorpb.MethodOptions{},
				},
			},
		},
	}
	return file
}

func newBrokenParentPatternFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("broken_parent_pattern.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage(
				"WeirdChild",
				"library.googleapis.com/WeirdChild",
				[]string{"publishers/{publisher}/{child}"},
			),
			newSimpleMessage("ListWeirdChildrenRequest", stringField("parent", 1)),
		},
	}
}

func newListBooksRequestMessage() *descriptorpb.DescriptorProto {
	message := newSimpleMessage("ListBooksRequest", stringField("parent", 1))
	message.Field = append(message.Field, &descriptorpb.FieldDescriptorProto{
		Name:    proto.String("page_size"),
		Number:  proto.Int32(2),
		Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:    descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
		Options: &descriptorpb.FieldOptions{},
	})
	return message
}

func newSimpleMessage(
	name string,
	fields ...*descriptorpb.FieldDescriptorProto,
) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name:    proto.String(name),
		Field:   fields,
		Options: &descriptorpb.MessageOptions{},
	}
}

func newResourceReferenceOnlyFile() *descriptorpb.FileDescriptorProto {
	field := stringField("parent", 1)
	field.Options = &descriptorpb.FieldOptions{}
	proto.SetExtension(
		field.Options,
		annotationspb.E_ResourceReference,
		&annotationspb.ResourceReference{ChildType: "library.googleapis.com/Book"},
	)

	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("reference_only.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String(
				"github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1",
			),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newSimpleMessage("ReferenceOnlyRequest", field),
		},
	}
}

func stringField(name string, number int32) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:   proto.String(name),
		Number: proto.Int32(number),
		Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
	}
}

func int32Field(name string, number int32) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:   proto.String(name),
		Number: proto.Int32(number),
		Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
	}
}
