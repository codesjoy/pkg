module github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason

go 1.25.7

require (
	github.com/codesjoy/pkg/proto/codesjoy/reason v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260311181403-84a4fc48630c
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/codesjoy/pkg/proto/codesjoy/reason => ../../proto/codesjoy/reason
