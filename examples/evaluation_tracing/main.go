package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema(map[string]wirefilter.Type{
		"http.host":   wirefilter.TypeString,
		"http.status": wirefilter.TypeInt,
		"http.method": wirefilter.TypeString,
	})

	filter, err := wirefilter.Compile(
		`(http.host == "example.com" or http.host == "test.com") and http.status >= 400 and http.method == "POST"`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext(map[string]wirefilter.Value{
		"http.host":   wirefilter.StringValue("example.com"),
		"http.status": wirefilter.IntValue(200),
		"http.method": wirefilter.StringValue("POST"),
	}).
		EnableTrace()

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Result: %v\n\n", result)

	trace := ctx.Trace()

	traceJSON, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Evaluation trace:\n%s\n", traceJSON)
}
