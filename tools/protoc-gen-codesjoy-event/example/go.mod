module github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-event/example

go 1.25.7

require (
	github.com/codesjoy/pkg/basic/xevent v0.0.0-00010101000000-000000000000
	github.com/codesjoy/pkg/proto/codesjoy/ddd v0.0.0-00010101000000-000000000000
	google.golang.org/protobuf v1.36.11
)

replace github.com/codesjoy/pkg/basic/xevent => ../../../basic/xevent

replace github.com/codesjoy/pkg/proto/codesjoy/ddd => ../../../proto/codesjoy/ddd
