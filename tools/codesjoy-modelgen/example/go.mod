module github.com/codesjoy/pkg/tools/codesjoy-modelgen/example

go 1.25.7

replace github.com/codesjoy/pkg/tools/codesjoy-modelgen => ..

require (
	github.com/codesjoy/pkg/basic/aipsql v0.0.0-20260226065627-6a5d82b2020b
	gorm.io/plugin/soft_delete v1.2.1
)

require (
	github.com/alecthomas/participle/v2 v2.1.4 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	golang.org/x/text v0.30.0 // indirect
	gorm.io/gorm v1.25.12 // indirect
)
