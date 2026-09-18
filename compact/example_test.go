package compact_test

import (
	"context"
	"fmt"
	"os"

	"github.com/muratmirgun/compact-engine/archive"
	"github.com/muratmirgun/compact-engine/compact"
	"github.com/muratmirgun/compact-engine/token"
)

func ExampleEngine_Compact() {
	dir, err := os.MkdirTemp("", "compact-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	store, err := archive.New(dir)
	if err != nil {
		panic(err)
	}
	counter, err := token.New("o200k_base")
	if err != nil {
		panic(err)
	}
	engine, err := compact.New(compact.NewReplay(nil), counter, store)
	if err != nil {
		panic(err)
	}
	result, err := engine.Compact(context.Background(), compact.Request{
		Goal: "Fix cache invalidation", TargetTokens: 1000,
		Messages: []compact.Message{{ID: "u1", Role: "user", Text: "Fix cache invalidation."}},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Status, result.BudgetMet, result.Stats.Scorer)
	// Output: unchanged true replay
}
