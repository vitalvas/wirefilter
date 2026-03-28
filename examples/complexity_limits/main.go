package main

import (
	"fmt"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema().
		AddField("http.host", wirefilter.TypeString).
		AddField("http.status", wirefilter.TypeInt).
		AddField("http.method", wirefilter.TypeString).
		SetMaxDepth(5).
		SetMaxNodes(10)

	// This simple expression compiles successfully
	simple, err := wirefilter.Compile(
		`http.host == "example.com" and http.status >= 400`,
		schema,
	)
	if err != nil {
		fmt.Printf("Simple filter failed: %v\n", err)
	} else {
		ctx := wirefilter.NewExecutionContext().
			SetStringField("http.host", "example.com").
			SetIntField("http.status", 500)

		result, err := simple.Execute(ctx)
		if err != nil {
			fmt.Printf("Execution error: %v\n", err)
		} else {
			fmt.Printf("Simple filter result: %v\n", result)
		}
	}

	// This deeply nested expression exceeds the max depth limit
	_, err = wirefilter.Compile(
		`(((((http.host == "a" and http.status == 1) or http.method == "GET") and http.host == "b") or http.status == 2) and http.method == "POST") or http.host == "c"`,
		schema,
	)
	if err != nil {
		fmt.Printf("Deeply nested filter rejected: %v\n", err)
	}

	// This expression exceeds the max node count limit
	nodeSchema := wirefilter.NewSchema().
		AddField("http.host", wirefilter.TypeString).
		AddField("http.status", wirefilter.TypeInt).
		SetMaxNodes(10)

	_, err = wirefilter.Compile(
		`http.host == "a" and http.host == "b" and http.host == "c" and http.host == "d" and http.host == "e" and http.status >= 100`,
		nodeSchema,
	)
	if err != nil {
		fmt.Printf("Complex filter rejected: %v\n", err)
	}
}
