package lexical_test

import (
	"errors"
	"fmt"

	"github.com/c360studio/semengine/graph/lexical"
)

func ExampleIndex() {
	ix := lexical.NewIndex()
	docs := map[string]lexical.Document{
		"acme.ops.docs.runbook.page.restart": {
			Name: "Restart the cooling pump",
			Body: "Stop it with docker compose down, then call withRetry until the pump reports ready.",
		},
		"acme.ops.docs.runbook.page.readings": {
			Name: "Pump telemetry",
			Body: "Pressure and flow readings for pump-7, sampled every second.",
		},
	}
	for id, doc := range docs {
		if err := ix.Put(id, doc); err != nil {
			fmt.Println(err)
			return
		}
	}

	for _, text := range []string{`pump retry`, `pump -telemetry`, `"compose down"`, `withretry`, `pump*`} {
		q, err := lexical.ParseQuery(text)
		if errors.Is(err, lexical.ErrReservedSyntax) {
			fmt.Printf("%s: reserved syntax\n", text)
			continue
		}
		if err != nil {
			fmt.Println(err)
			return
		}
		res, err := ix.Search(q, lexical.Options{Limit: 10})
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%s: %d matched", text, res.Matched)
		for _, hit := range res.Hits {
			fmt.Printf(" %s", hit.EntityID)
		}
		fmt.Println()
	}
	// Output:
	// pump retry: 2 matched acme.ops.docs.runbook.page.restart acme.ops.docs.runbook.page.readings
	// pump -telemetry: 1 matched acme.ops.docs.runbook.page.restart
	// "compose down": 1 matched acme.ops.docs.runbook.page.restart
	// withretry: 0 matched
	// pump*: reserved syntax
}
