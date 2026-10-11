package main

import (
	"fmt"
	"log"

	"go.vitalvas.com/wirefilter"
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

	ctx := wirefilter.NewExecutionContext(map[string]wirefilter.Value{
		"http.host":   wirefilter.StringValue("api.example.com"),
		"http.method": wirefilter.StringValue("GET"),
		"http.path":   wirefilter.StringValue("/api/v2/users"),
		"http.status": wirefilter.IntValue(200),
		"http.secure": wirefilter.BoolValue(true),
	})

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
