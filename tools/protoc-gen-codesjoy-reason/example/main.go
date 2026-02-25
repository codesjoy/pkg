package main

import (
	"fmt"

	userv1 "github.com/codesjoy/pkg/tools/protoc-gen-codesjoy-reason/example/protogen/codesjoy/example/user/v1"
)

func main() {
	r := userv1.UserReason_USER_REASON_NOT_FOUND
	fmt.Printf("reason=%s domain=%s code=%s\n", r.Reason(), r.Domain(), r.Code().String())
}
