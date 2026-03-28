package main

import (
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema().
		AddField("user.id", wirefilter.TypeString).
		AddField("ip.src", wirefilter.TypeIP)

	filter, err := wirefilter.Compile(
		`$user_tier[user.id] == "premium" and ip.src in $allowed_ips[user.id]`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext().
		SetStringField("user.id", "user-42").
		SetIPField("ip.src", "10.0.1.5").
		SetTable("user_tier", map[string]string{
			"user-42": "premium",
			"user-99": "free",
		}).
		SetTableIPList("allowed_ips", map[string][]string{
			"user-42": {"10.0.0.0/8", "172.16.0.0/12"},
			"user-99": {"192.168.1.0/24"},
		})

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
