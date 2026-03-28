package main

import (
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema().
		AddMapField("http.headers", wirefilter.TypeString).
		AddMapField("scores", wirefilter.TypeFloat)

	filter, err := wirefilter.Compile(
		`http.headers["content-type"] == "application/json" and scores["risk"] > 0.8`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext().
		SetMapField("http.headers", map[string]string{
			"content-type":  "application/json",
			"authorization": "Bearer token123",
			"x-request-id":  "abc-def-ghi",
		}).
		SetMapFieldValues("scores", map[string]wirefilter.Value{
			"risk":       wirefilter.FloatValue(0.95),
			"confidence": wirefilter.FloatValue(0.87),
		})

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
