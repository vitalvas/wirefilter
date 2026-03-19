package main

import (
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema(
		map[string]wirefilter.Type{
			"ip.src":   wirefilter.TypeIP,
			"port.dst": wirefilter.TypeInt,
			"protocol": wirefilter.TypeString,
			"tags":     wirefilter.TypeArray,
		},
	)

	filter, err := wirefilter.Compile(`
		ip.src in "10.0.0.0/8" and
		port.dst in {80, 443, 8080..8090} and
		protocol == "tcp" and
		tags contains "production"
	`, schema)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext().
		SetIPField("ip.src", "10.1.2.3").
		SetIntField("port.dst", 443).
		SetStringField("protocol", "tcp").
		SetArrayField("tags", []string{"production", "v2"})

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
