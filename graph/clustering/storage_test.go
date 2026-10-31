package clustering

import (
	"testing"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestNewNATSCommunityStorageRefusesNil: a nil store is refused at construction rather
// than panicking at the first read.
func TestNewNATSCommunityStorageRefusesNil(t *testing.T) {
	storage, err := NewNATSCommunityStorage(nil)
	if storage != nil || !errs.IsInvalid(err) {
		t.Fatalf("NewNATSCommunityStorage(nil) = %v, %v; want nil and an invalid error", storage, err)
	}
}

// TestGetAllCommunitiesRefusesNilContext: a nil context is refused before any read.
func TestGetAllCommunitiesRefusesNilContext(t *testing.T) {
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	// The bucket is nil: a read through it would panic, so a refusal proves none happened.
	storage, err := NewNATSCommunityStorage(client.NewKVStore(nil))
	if err != nil {
		t.Fatal(err)
	}
	//nolint:staticcheck // SA1012: the nil context is the input under test.
	got, err := storage.GetAllCommunities(nil)
	if got != nil || !errs.IsInvalid(err) {
		t.Fatalf("GetAllCommunities(nil) = %v, %v; want nil and an invalid error", got, err)
	}
}

// TestCommunityKey: a record's key is its level, a dot, and its group ID (pin
// storage.go:518-520).
func TestCommunityKey(t *testing.T) {
	if got, want := communityKey(2, "acme.dep1"), "2.acme.dep1"; got != want {
		t.Fatalf("communityKey = %q, want %q", got, want)
	}
}
