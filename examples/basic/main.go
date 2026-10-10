package main

import (
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema(map[string]wirefilter.Type{
		"http.host":   wirefilter.TypeString,
		"http.status": wirefilter.TypeInt,
	})

	filter, err := wirefilter.Compile(`http.host == "example.com" and http.status >= 400`, schema)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext(map[string]wirefilter.Value{
		"http.host":   wirefilter.StringValue("example.com"),
		"http.status": wirefilter.IntValue(500),
	})

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
