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
	features, err := parseFeatureSet("resources,field-behavior,method_signature")
	require.NoError(t, err)
	require.True(t, features.resources)
	require.True(t, features.fieldBehavior)
	require.True(t, features.methodSignature)

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

	require.NoError(t, generateFiles(gen, featureSet{
		resources:       true,
		fieldBehavior:   true,
		methodSignature: true,
	}))

	files := generatedFilesBySuffix(gen, "_google_aip.pb.go")
	require.Len(t, files, 2)

	publisherContent := generatedFileContent(gen, "publisher_google_aip.pb.go")
	require.NotEmpty(t, publisherContent)
	assert.Contains(t, publisherContent, `const PublisherNamePattern = "publishers/{publisher}"`)
	assert.Contains(t, publisherContent, "googleaip.MustCompilePattern(PublisherNamePattern)")
	assert.Contains(t, publisherContent, "func (x *Publisher) ParseName() (googleaip.ResourceMatch, error)")
	assert.Contains(t, publisherContent, "func (x *Publisher) ValidateName() error")
	assert.Contains(t, publisherContent, "func (x *Publisher) FillName(values map[string]string) error")
	assert.Contains(t, publisherContent, "return x.FillNameWithPattern(PublisherNamePattern, values)")
	assert.NotContains(t, publisherContent, `googleaip.MustCompilePattern("publishers/{publisher}")`)
	assert.Contains(t, publisherContent, "func (GetBookRequest) GoogleAIPResourceReference(fieldName string) (googleaip.ResourceReferenceMetadata, bool)")
	assert.Contains(t, publisherContent, "func (GetBookRequest) GoogleAIPFieldBehaviors(fieldName string) []annotations.FieldBehavior")
	assert.Contains(t, publisherContent, "func (GetBookRequest) GoogleAIPRequiredFields() []string")
	assert.Contains(t, publisherContent, "func LibraryServiceGoogleAIPMethodSignatures(methodName string) [][]string")
	assert.NotContains(t, publisherContent, "func GoogleAIPResourceReference(messageFullName, fieldName string)")
	assert.NotContains(t, publisherContent, "func GoogleAIPFieldBehaviors(messageFullName, fieldName string)")
	assert.NotContains(t, publisherContent, "func GoogleAIPRequiredFields(messageFullName string)")
	assert.NotContains(t, publisherContent, "func GoogleAIPMethodSignatures(serviceFullName, methodName string)")
	assert.NotContains(t, publisherContent, "func (x *Book) FillNameWithPattern(pattern string, values map[string]string) error")
	assert.Contains(t, publisherContent, `"name": googleaip.ResourceReferenceMetadata{`)

	bookContent := generatedFileContent(gen, "book_google_aip.pb.go")
	require.NotEmpty(t, bookContent)
	assert.Contains(t, bookContent, `BookNamePattern1 = "publishers/{publisher}/books/{book}"`)
	assert.Contains(t, bookContent, `BookNamePattern2 = "archives/{archive}/books/{book}"`)
	assert.Contains(t, bookContent, "googleaip.MustCompilePattern(BookNamePattern1)")
	assert.Contains(t, bookContent, "googleaip.MustCompilePattern(BookNamePattern2)")
	assert.NotContains(t, bookContent, `googleaip.MustCompilePattern("publishers/{publisher}/books/{book}")`)
	assert.Contains(t, bookContent, "func (x *Book) FillNameWithPattern(pattern string, values map[string]string) error")
	assert.NotContains(t, bookContent, "func (x *Book) FillName(values map[string]string) error")
	assert.Contains(t, bookContent, "nil *Book receiver")
	assert.NotContains(t, bookContent, "func (GetBookRequest) GoogleAIPResourceReference(fieldName string)")
	assert.NotContains(t, bookContent, "func LibraryServiceGoogleAIPMethodSignatures(methodName string) [][]string")
	assert.NotContains(t, bookContent, "library.googleapis.com/Archive")
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
	assert.Contains(t, content, "func (x *Publisher) ParseName() (googleaip.ResourceMatch, error)")
	assert.Contains(t, content, "func (GetBookRequest) GoogleAIPResourceReference(fieldName string) (googleaip.ResourceReferenceMetadata, bool)")
	assert.NotContains(t, content, "func GoogleAIPLookupResource(resourceType string)")
	assert.NotContains(t, content, "func (GetBookRequest) GoogleAIPFieldBehaviors(fieldName string) []annotations.FieldBehavior")
	assert.NotContains(t, content, "func LibraryServiceGoogleAIPMethodSignatures(methodName string) [][]string")
}

func TestGenerateFilesSkipsWhenNoAnnotations(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"plain.proto"},
		[]*descriptorpb.FileDescriptorProto{newPlainFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{
		resources:       true,
		fieldBehavior:   true,
		methodSignature: true,
	}))
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

func TestGenerateFilesUsesConfiguredNameField(t *testing.T) {
	gen, err := newTestPluginWithFiles(
		[]string{"shelf.proto"},
		[]*descriptorpb.FileDescriptorProto{newNamedResourceFile()},
	)
	require.NoError(t, err)

	require.NoError(t, generateFiles(gen, featureSet{resources: true}))

	content := generatedFileContent(gen, "shelf_google_aip.pb.go")
	require.NotEmpty(t, content)
	assert.Contains(t, content, "return googleAIPResourceShelfDescriptor.Validate(x.ResourceName)")
	assert.Contains(t, content, "return googleAIPResourceShelfDescriptor.Parse(x.ResourceName)")
	assert.Contains(t, content, "x.ResourceName = formatted")
	assert.Contains(t, content, `const ShelfNamePattern = "shelves/{shelf}"`)
	assert.Contains(t, content, "return x.FillNameWithPattern(ShelfNamePattern, values)")
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
			GoPackage: proto.String("github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1"),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage("Publisher", "library.googleapis.com/Publisher", []string{"publishers/{publisher}"}),
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

	proto.SetExtension(file.Service[0].Method[0].Options, annotationspb.E_MethodSignature, []string{"name, view"})
	return file
}

func newBookFile() *descriptorpb.FileDescriptorProto {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("book.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1"),
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

	proto.SetExtension(file.Options, annotationspb.E_ResourceDefinition, []*annotationspb.ResourceDescriptor{
		{
			Type:     "library.googleapis.com/Archive",
			Pattern:  []string{"archives/{archive}"},
			Plural:   "archives",
			Singular: "archive",
		},
	})
	return file
}

func newFileDefinitionsOnlyFile() *descriptorpb.FileDescriptorProto {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("definitions.proto"),
		Package: proto.String("codesjoy.example.library.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1"),
		},
	}

	proto.SetExtension(file.Options, annotationspb.E_ResourceDefinition, []*annotationspb.ResourceDescriptor{
		{
			Type:     "library.googleapis.com/Archive",
			Pattern:  []string{"archives/{archive}"},
			Plural:   "archives",
			Singular: "archive",
		},
	})
	return file
}

func newPlainFile() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("plain.proto"),
		Package: proto.String("codesjoy.example.plain.v1"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/plain/v1;plainv1"),
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

func newResourceMessage(name, resourceType string, patterns []string) *descriptorpb.DescriptorProto {
	return newResourceMessageWithNameField(name, resourceType, patterns, "name", stringField("name", 1))
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
			GoPackage: proto.String("github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1"),
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
			GoPackage: proto.String("github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1"),
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
			GoPackage: proto.String("github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1"),
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
			GoPackage: proto.String("github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1;libraryv1"),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			newResourceMessage("DuplicatePublisher", "library.googleapis.com/Publisher", []string{"duplicate_publishers/{publisher}"}),
		},
	}
}

func newGetBookRequestMessage() *descriptorpb.DescriptorProto {
	message := &descriptorpb.DescriptorProto{
		Name: proto.String("GetBookRequest"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:    proto.String("name"),
				Number:  proto.Int32(1),
				Label:   descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:    descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Options: &descriptorpb.FieldOptions{},
			},
		},
		Options: &descriptorpb.MessageOptions{},
	}

	proto.SetExtension(
		message.Field[0].Options,
		annotationspb.E_ResourceReference,
		&annotationspb.ResourceReference{Type: "library.googleapis.com/Book"},
	)
	proto.SetExtension(
		message.Field[0].Options,
		annotationspb.E_FieldBehavior,
		[]annotationspb.FieldBehavior{
			annotationspb.FieldBehavior_REQUIRED,
			annotationspb.FieldBehavior_IDENTIFIER,
		},
	)
	return message
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
