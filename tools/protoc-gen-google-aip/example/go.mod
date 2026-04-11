module github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example

go 1.25.7

require (
	google.golang.org/genproto/googleapis/api v0.0.0-20250825161204-c5933d9347a5
	google.golang.org/protobuf v1.36.11
)

replace github.com/codesjoy/pkg/tools/protoc-gen-google-aip => ..
