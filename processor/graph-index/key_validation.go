package graphindex

import (
	"github.com/c360studio/semengine/natsclient"
	semtypes "github.com/c360studio/semengine/pkg/types"
)

// validateEntityLiteralKey refuses an entity ID that is not a literal six-part ID or not a
// literal NATS KV key, before any index row is listed, written or deleted for it.
func validateEntityLiteralKey(entityID string) error {
	if err := semtypes.ValidateEntityID(entityID); err != nil {
		return err
	}
	return natsclient.ValidateKVLiteralKey(entityID)
}
