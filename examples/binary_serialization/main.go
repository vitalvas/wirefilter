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

	filter, err := wirefilter.Compile(
		`http.host == "example.com" and http.status >= 400`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Serialize the compiled filter to binary
	data, err := filter.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Serialized filter: %d bytes\n", len(data))
	fmt.Printf("Filter hash: %s\n", filter.Hash())

	// Deserialize the filter from binary
	restored := &wirefilter.Filter{}

	if err := restored.UnmarshalBinary(data); err != nil {
		log.Fatal(err)
	}

	// Execute the restored filter
	ctx := wirefilter.NewExecutionContext(map[string]wirefilter.Value{
		"http.host":   wirefilter.StringValue("example.com"),
		"http.status": wirefilter.IntValue(500),
	})

	result, err := restored.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Restored filter result: %v\n", result)
	fmt.Printf("Hashes match: %v\n", filter.Hash() == restored.Hash())
}
