package objectstore

import "log/slog"

// defaultBucketName is the bucket a Config with no BucketName uses.
const defaultBucketName = "MESSAGES"

// Config configures a Store.
type Config struct {
	// BucketName is the NATS ObjectStore bucket. Empty means "MESSAGES".
	BucketName string `json:"bucket_name"`

	// InstanceName is stamped into every StorageReference.StorageInstance the
	// Store returns: the name a storeregistry.Registry resolves back to this
	// Store (ADR-063). Empty means BucketName. Set by the owning code, not by an
	// operator.
	InstanceName string `json:"-"`

	// Logger receives the retention guard's warning when construction strips a
	// TTL or size limit from the bucket. Nil means slog.Default().
	Logger *slog.Logger `json:"-"`
}

// withDefaults returns c with every empty field set to its default.
func (c Config) withDefaults() Config {
	if c.BucketName == "" {
		c.BucketName = defaultBucketName
	}
	if c.InstanceName == "" {
		c.InstanceName = c.BucketName
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}
