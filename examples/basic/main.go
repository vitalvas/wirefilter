package main

import (
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema().
		AddField("http.host", wirefilter.TypeString).
		AddField("http.status", wirefilter.TypeInt)

	filter, err := wirefilter.Compile(`http.host == "example.com" and http.status >= 400`, schema)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext().
		SetStringField("http.host", "example.com").
		SetIntField("http.status", 500)

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
