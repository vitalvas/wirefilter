package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"go.vitalvas.com/wirefilter"
)

func main() {
	schema := wirefilter.NewSchema(map[string]wirefilter.Type{
		"http.host": wirefilter.TypeString,
		"user.role": wirefilter.TypeString,
	}).
		RegisterFunction("normalize_host", wirefilter.TypeString, []wirefilter.Type{wirefilter.TypeString}).
		RegisterFunction("has_permission", wirefilter.TypeBool, []wirefilter.Type{wirefilter.TypeString, wirefilter.TypeString})

	filter, err := wirefilter.Compile(
		`normalize_host(http.host) == "example.com" and has_permission(user.role, "admin")`,
		schema,
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := wirefilter.NewExecutionContext(map[string]wirefilter.Value{
		"http.host": wirefilter.StringValue("EXAMPLE.COM"),
		"user.role": wirefilter.StringValue("superadmin"),
	}).
		SetFunc("normalize_host", func(_ context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
			host := string(args[0].(wirefilter.StringValue))
			return wirefilter.StringValue(strings.ToLower(host)), nil
		}).
		SetFunc("has_permission", func(_ context.Context, args []wirefilter.Value) (wirefilter.Value, error) {
			role := string(args[0].(wirefilter.StringValue))
			required := string(args[1].(wirefilter.StringValue))
			permissions := map[string][]string{
				"superadmin": {"admin", "write", "read"},
				"editor":     {"write", "read"},
				"viewer":     {"read"},
			}
			for _, perm := range permissions[role] {
				if perm == required {
					return wirefilter.BoolValue(true), nil
				}
			}
			return wirefilter.BoolValue(false), nil
		})

	result, err := filter.Execute(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
}
