package objectstore_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/storage/objectstore"
)

// ExampleNewStoreWithConfig stores, reads, lists and deletes one object. It has
// no Output comment because it needs a running NATS server with JetStream, so
// `go test` compiles it and does not run it.
func ExampleNewStoreWithConfig() {
	ctx := context.Background()

	// NATS_URL is the URL of a NATS server with JetStream enabled.
	client, err := natsclient.NewClient(os.Getenv("NATS_URL"))
	if err != nil {
		log.Fatal(err)
	}
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer client.Close(ctx)

	// The bucket is created if it is missing. The Store has no Close.
	store, err := objectstore.NewStoreWithConfig(ctx, client, objectstore.Config{BucketName: "MESSAGES"})
	if err != nil {
		log.Fatal(err)
	}

	// A second Put to the same key replaces the object.
	if err := store.Put(ctx, "events/a", []byte("hello")); err != nil {
		log.Fatal(err)
	}

	data, err := store.Get(ctx, "events/a")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))

	// List returns the matching names sorted.
	keys, err := store.List(ctx, "events/")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(keys)

	// Delete removes the object; deleting a key with no object also returns nil.
	if err := store.Delete(ctx, "events/a"); err != nil {
		log.Fatal(err)
	}

	// A missing object matches storage.ErrObjectNotFound.
	if _, err := store.Get(ctx, "events/a"); errors.Is(err, storage.ErrObjectNotFound) {
		fmt.Println("gone")
	}
}
