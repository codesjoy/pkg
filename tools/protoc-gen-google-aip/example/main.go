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
	request := libraryv1.GetBookRequest{Name: book.GetName()}
	bookParsed, _ := libraryv1.ParseBookName(request.GetName())
	bookNameValid := libraryv1.ValidateBookName(request.GetName()) == nil
	listRequest := &libraryv1.ListBooksRequest{Parent: "publishers/pub-1"}
	publisherParentParsed, _ := libraryv1.ParseBookParent(listRequest.Parent)
	publisherParentValid := libraryv1.ValidateBookParent(listRequest.Parent) == nil
	createParent := "archives/arc-1"
	archiveParentParsed, _ := libraryv1.ParseBookParent(createParent)
	archiveParentValid := libraryv1.ValidateBookParent(createParent) == nil

	fmt.Printf(
		"publisher=%s pattern=%s name_valid=%t book_publisher=%s book_id=%s book_archive=%q parent_pub_type=%s parent_pub_publisher=%s parent_pub_archive=%q parent_pub_valid=%t parent_archive_type=%s parent_archive_publisher=%q parent_archive_archive=%s parent_archive_valid=%t\n",
		publisher.GetName(),
		bookParsed.Pattern,
		bookNameValid,
		bookParsed.Publisher,
		bookParsed.Book,
		bookParsed.Archive,
		publisherParentParsed.DescriptorType,
		publisherParentParsed.Publisher,
		publisherParentParsed.Archive,
		publisherParentValid,
		archiveParentParsed.DescriptorType,
		archiveParentParsed.Publisher,
		archiveParentParsed.Archive,
		archiveParentValid,
	)
}
