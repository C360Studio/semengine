package storagereport

import (
	"context"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
)

// The storage reporting rows of natsclient's test of the same name, moved with the code: every
// exported, error-returning storagereport entry that takes a context refuses a nil one, without
// panicking, with the invalid-data error the rest of natsclient returns (Codex F32).
func TestExportedNilContextRefusedOnEveryOtherEntry(t *testing.T) {
	collector := accountTestCollector(t, (&accountAwareLister{}).source())
	publisher := newTestPublisher(t, newFakeReportStore(1), defaultSource())

	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		{"StorageInventoryCollector.Collect", func(ctx context.Context) error { _, err := collector.Collect(ctx); return err }},
		{"Publisher.Publish", func(ctx context.Context) error {
			_, err := publisher.Publish(ctx, StorageInventory{})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("nil context panicked: %v", p)
				}
			}()
			var nilCtx context.Context
			err := tc.call(nilCtx)
			switch {
			case err == nil:
				t.Error("nil context accepted")
			case !errs.IsInvalid(err):
				t.Errorf("nil context: err = %v, want an invalid-data refusal", err)
			}
		})
	}
}
