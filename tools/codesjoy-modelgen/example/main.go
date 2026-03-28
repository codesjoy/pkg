// Copyright 2022 The codesjoy Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import "fmt"

func main() {
	fmt.Println("codesjoy-modelgen example (PostgreSQL)")
	fmt.Println("README: ./README.md")
	fmt.Println("Start DB: docker compose up -d")
	fmt.Println("Generate: go run ../ --dsn ... --schema public")
	fmt.Println("          --tables users --package demo --out-dir ./output")
	fmt.Println("          --gen-aipsql=true --timestamp-mode unix_nano")
	fmt.Println("          --override ./override.yaml")
}
