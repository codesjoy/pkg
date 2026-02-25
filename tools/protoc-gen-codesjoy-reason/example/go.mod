module github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason/example

go 1.25.7

require (
	github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason v0.0.0-00010101000000-000000000000
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5
	google.golang.org/protobuf v1.36.11
)

replace github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason => ..
