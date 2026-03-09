package main

import (
	"fmt"

	libraryv1 "github.com/codesjoy/pkg/tools/protoc-gen-google-aip/example/protogen/codesjoy/example/library/v1"
)

func main() {
	publisher := &libraryv1.Publisher{}
	_ = publisher.FillName(map[string]string{"publisher": "pub-1"})

	book := &libraryv1.Book{}
	_ = book.FillNameWithPattern(libraryv1.BookNamePattern1, map[string]string{
		"publisher": "pub-1",
		"book":      "book-1",
	})
	bookMatch, _ := book.ParseName()
	request := libraryv1.GetBookRequest{}
	ref, _ := request.GoogleAIPResourceReference("name")
	required := request.GoogleAIPRequiredFields()
	signatures := libraryv1.LibraryServiceGoogleAIPMethodSignatures("GetBook")

	fmt.Printf(
		"publisher=%s pattern=%s ref_type=%s required=%v signatures=%v book_vars=%v\n",
		publisher.GetName(),
		bookMatch.Pattern,
		ref.Type,
		required,
		signatures,
		bookMatch.Values,
	)
}
