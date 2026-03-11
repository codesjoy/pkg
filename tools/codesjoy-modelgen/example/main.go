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
