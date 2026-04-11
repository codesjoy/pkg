module github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason/example

go 1.25.7

require (
	github.com/codesjoy/pkg/proto/codesjoy/reason v0.0.0-20260315175100-21ea1546c9c2
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260311181403-84a4fc48630c
	google.golang.org/protobuf v1.36.11
)

replace github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason => ..
