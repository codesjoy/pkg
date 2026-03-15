module github.com/codesjoy/pkg/basic/xevent/nats

go 1.25.7

replace (
	github.com/codesjoy/pkg/basic/xevent => ../
	github.com/codesjoy/pkg/basic/xnats => ../../xnats
)

require (
	github.com/codesjoy/pkg/basic/xevent v0.0.0-00010101000000-000000000000
	github.com/codesjoy/pkg/basic/xnats v0.0.0-00010101000000-000000000000
	github.com/nats-io/nats.go v1.49.0
)

require (
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/nats-io/nkeys v0.4.15 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
)
