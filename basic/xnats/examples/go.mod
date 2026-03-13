module github.com/codesjoy/pkg/basic/xnats/examples

go 1.25.7

require (
	github.com/codesjoy/pkg/basic/xnats v0.0.0
	github.com/nats-io/nats.go v1.49.0
)

require (
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/nats-io/nkeys v0.4.12 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
)

replace github.com/codesjoy/pkg/basic/xnats => ../
