package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.vitalvas.com/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema(map[string]wirefilter.Type{
		"http.host": wirefilter.TypeString,
	}).
		RegisterFunction("slow_lookup", wirefilter.TypeBool, []wirefilter.Type{wirefilter.TypeString})

	filter, err := wirefilter.Compile(
		`slow_lookup(http.host)`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Set a 100ms timeout for filter evaluation
	goCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ctx := wirefilter.NewExecutionContext(map[string]wirefilter.Value{
		"http.host": wirefilter.StringValue("example.com"),
	}).
		WithContext(goCtx).
		SetFunc("slow_lookup", func(goCtx context.Context, _ []wirefilter.Value) (wirefilter.Value, error) {
			// Simulate a slow external lookup that respects context cancellation
			select {
			case <-time.After(5 * time.Second):
				return wirefilter.BoolValue(true), nil
			case <-goCtx.Done():
				return nil, goCtx.Err()
			}
		})

	result, err := filter.Execute(ctx)
	if err != nil {
		fmt.Printf("Filter timed out as expected: %v\n", err)
		return
	}

	fmt.Println(result)
}
