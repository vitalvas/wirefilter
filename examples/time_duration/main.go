package main

import (
	"fmt"
	"log"
	"time"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema().
		AddField("event.time", wirefilter.TypeTime).
		AddField("session.duration", wirefilter.TypeDuration).
		AddField("event.name", wirefilter.TypeString)

	// Find events that happened in the last hour with sessions longer than 30 minutes
	filter, err := wirefilter.Compile(
		`event.time > now() - 1h and session.duration > 30m and event.name == "checkout"`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	fixedNow := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	ctx := wirefilter.NewExecutionContext().
		SetTimeField("event.time", fixedNow.Add(-20*time.Minute)).
		SetDurationField("session.duration", 45*time.Minute).
		SetStringField("event.name", "checkout").
		WithNow(func() time.Time { return fixedNow })

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
