package main

import (
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema(
		map[string]wirefilter.Type{
			"http.host":   wirefilter.TypeString,
			"http.method": wirefilter.TypeString,
			"http.path":   wirefilter.TypeString,
			"http.status": wirefilter.TypeInt,
			"http.secure": wirefilter.TypeBool,
		},
	)

	filter, err := wirefilter.Compile(`
		(http.host == "api.example.com" or http.host wildcard "*.test.com") and
		http.method == "GET" and
		http.path matches "^/api/v[0-9]+/" and
		http.status in {200..299} and
		http.secure == true
	`, schema)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext().
		SetStringField("http.host", "api.example.com").
		SetStringField("http.method", "GET").
		SetStringField("http.path", "/api/v2/users").
		SetIntField("http.status", 200).
		SetBoolField("http.secure", true)

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
