package wirefilter

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllCompletePredicate(t *testing.T) {
	tests := []struct {
		expr string
		ctx  *ExecutionContext
		want bool
	}{
		{`all(tags[*] wildcard "prod*")`, NewExecutionContext().SetArrayField("tags", []string{"dev"}), false},
		{`all(tags[*] wildcard "prod*")`, NewExecutionContext().SetArrayField("tags", []string{"prod", "PROD-api"}), true},
		{`all(tags[*] strict wildcard "prod*")`, NewExecutionContext().SetArrayField("tags", []string{"prod", "PROD-api"}), false},
		{`all(ports[*] > 0 and ports[*] < 65536)`, NewExecutionContext().SetIntArrayField("ports", []int64{-1, 70000}), false},
		{`all(ports[*] > 0 and ports[*] < 65536)`, NewExecutionContext().SetIntArrayField("ports", []int64{1, 65535}), true},
		{`all(ports[*] < 0 or ports[*] > 10)`, NewExecutionContext().SetIntArrayField("ports", []int64{-1, 11}), true},
		{`all(ports[*]/2 >= 1)`, NewExecutionContext().SetIntArrayField("ports", []int64{2, 4}), true},
		{`all(ports[*] in {1..100})`, NewExecutionContext().SetIntArrayField("ports", []int64{1, 100}), true},
		{`all(ports[*] in {1..100})`, NewExecutionContext().SetIntArrayField("ports", []int64{1, 101}), false},
		{`all($lookup[tags[*]] == "allowed")`, NewExecutionContext().SetArrayField("tags", []string{"prod", "api"}).SetTable("lookup", map[string]string{"prod": "allowed", "api": "allowed"}), true},
		{`all(lower(tags[*]) == "prod")`, NewExecutionContext().SetArrayField("tags", []string{"prod", "PROD"}), true},
		{`all("prod" == tags[*])`, NewExecutionContext().SetArrayField("tags", []string{"prod", "dev"}), false},
		{`all(tags[*] not in {"dev"})`, NewExecutionContext().SetArrayField("tags", []string{"prod", "dev"}), false},
		{`all(tags[*] == "prod")`, NewExecutionContext().SetArrayField("tags", nil), false},
		{`all(tags[*] == "prod")`, NewExecutionContext(), false},
		{`all(flags[*])`, NewExecutionContext().SetField("flags", ArrayValue{BoolValue(true), BoolValue(false)}), false},
		{`all(flags[*])`, NewExecutionContext().SetField("flags", ArrayValue{BoolValue(true), BoolValue(true)}), true},
		{`all(tags[*] == "prod" and all(ports[*] > 0))`, NewExecutionContext().SetArrayField("tags", []string{"prod"}).SetIntArrayField("ports", []int64{1, 2}), true},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%t", tc.expr, tc.want), func(t *testing.T) {
			requireMatch(t, tc.expr, tc.ctx, tc.want)
		})
	}
	t.Run("short circuit", func(t *testing.T) {
		calls := 0
		ctx := NewExecutionContext().SetIntArrayField("ports", []int64{-1}).
			SetFunc("fail", func(_ context.Context, _ []Value) (Value, error) {
				calls++
				return nil, fmt.Errorf("unexpected call")
			})
		requireMatch(t, `all(ports[*] > 0 and fail())`, ctx, false)
		require.Zero(t, calls)
	})
	t.Run("independent arrays require explicit quantifiers", func(t *testing.T) {
		filter, err := Compile(`all(a[*] == b[*])`, nil)
		require.NoError(t, err)
		_, err = filter.Execute(NewExecutionContext().SetArrayField("a", []string{"x"}).SetArrayField("b", []string{"x"}))
		require.ErrorContains(t, err, "same array")
	})
	t.Run("compiled predicate remains immutable", func(t *testing.T) {
		filter, err := Compile(`all(ports[*] > 0 and ports[*] < 100)`, nil)
		require.NoError(t, err)
		before, err := filter.MarshalBinary()
		require.NoError(t, err)
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Go(func() {
				want := i%2 == 0
				port := int64(-1)
				if want {
					port = 1
				}
				got, err := filter.Execute(NewExecutionContext().SetIntArrayField("ports", []int64{port}))
				assert.NoError(t, err)
				assert.Equal(t, want, got)
			})
		}
		wg.Wait()
		after, err := filter.MarshalBinary()
		require.NoError(t, err)
		require.Equal(t, before, after)
	})
}
