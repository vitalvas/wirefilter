package main

import (
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema().
		AddArrayField("tags", wirefilter.TypeString).
		AddArrayField("scores", wirefilter.TypeInt)

	// Check if any tag matches and all scores are above a threshold
	filter, err := wirefilter.Compile(
		`tags[*] == "critical" and scores[*] > 50`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext().
		SetArrayField("tags", []string{"info", "critical", "security"}).
		SetIntArrayField("scores", []int64{80, 95, 72})

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Any critical tag and any score > 50: %v\n", result)

	// Use === to check if ALL elements match (on untyped array)
	allSchema := wirefilter.NewSchema().
		AddField("labels", wirefilter.TypeArray)

	allFilter, err := wirefilter.Compile(
		`labels === "active"`,
		allSchema,
	)
	if err != nil {
		log.Fatal(err)
	}

	allCtx := wirefilter.NewExecutionContext().
		SetArrayField("labels", []string{"active", "active", "active"})

	allResult, err := allFilter.Execute(allCtx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("All labels equal 'active': %v\n", allResult)

	// Use !== to check if any element does NOT match
	anyNeFilter, err := wirefilter.Compile(
		`labels !== "active"`,
		allSchema,
	)
	if err != nil {
		log.Fatal(err)
	}

	mixedCtx := wirefilter.NewExecutionContext().
		SetArrayField("labels", []string{"active", "inactive", "active"})

	anyNeResult, err := anyNeFilter.Execute(mixedCtx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Any label not 'active': %v\n", anyNeResult)
}
