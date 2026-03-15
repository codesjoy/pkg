module github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-ddd

go 1.25.7

require (
	github.com/codesjoy/pkg/proto/codesjoy/ddd v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/codesjoy/pkg/proto/codesjoy/ddd => ../../proto/codesjoy/ddd
