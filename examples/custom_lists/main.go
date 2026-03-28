package main

import (
	"fmt"
	"log"

	"github.com/vitalvas/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema().
		AddField("http.host", wirefilter.TypeString).
		AddField("ip.src", wirefilter.TypeIP)

	filter, err := wirefilter.Compile(
		`http.host in $blocked_hosts or ip.src in $blocked_ips`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext().
		SetStringField("http.host", "malware.example.com").
		SetIPField("ip.src", "203.0.113.50").
		SetList("blocked_hosts", []string{
			"malware.example.com",
			"phishing.test.net",
			"spam.bad.org",
		}).
		SetIPList("blocked_ips", []string{
			"203.0.113.0/24",
			"198.51.100.0/24",
			"10.0.0.1",
		})

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
