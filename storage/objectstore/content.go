package objectstore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/types"
)

// contentType is the type of every object StoreContent writes: the body text.
const contentType = "text/plain; charset=utf-8"

// StoreContent stores the body of cs, the RawContent field its "body" role
// names, as raw UTF-8 bytes, and returns a reference to it: the Store's instance
// name, the object's key, text/plain and the body's size. Opening the reference
// returns the body; there is no envelope (ADR-063).
//
// It refuses, as invalid input and before writing anything, an invalid entity
// ID and any other content: a non-empty RawContent value under a field that is
// not the body's, or a non-empty binary field of a message.BinaryStorable. The error
// names the refused fields. Non-body text belongs in the graph as triples, which
// the producer emits. A payload with no content stores an empty object.
func (s *Store) StoreContent(ctx context.Context, cs message.ContentStorable) (*message.StorageReference, error) {
	if ctx == nil {
		return nil, nilContext("StoreContent")
	}
	if cs == nil {
		return nil, errs.WrapInvalid(errors.New("nil content"), "Store", "StoreContent", "missing content")
	}
	entityID := cs.EntityID()
	if err := types.ValidateEntityID(entityID); err != nil {
		return nil, errs.WrapInvalid(err, "Store", "StoreContent", "validate entity ID")
	}
	body, err := bodyOnly(cs)
	if err != nil {
		return nil, errs.WrapInvalid(err, "Store", "StoreContent", "check content")
	}

	start := time.Now()
	s.metrics.recordWriteOp("store_content")
	key := contentKey(entityID)
	data := []byte(body)
	if _, err := s.objects.PutBytes(ctx, key, data); err != nil {
		s.metrics.recordError("store_content")
		return nil, errs.WrapTransient(err, "Store", "StoreContent", "store content")
	}
	s.metrics.recordWriteLatency("store_content", start)

	return &message.StorageReference{
		StorageInstance: s.instanceName,
		Key:             key,
		ContentType:     contentType,
		Size:            int64(len(data)),
	}, nil
}

// bodyOnly returns the text cs's body role names, or an error naming every
// other non-empty text field and every non-empty binary field cs carries.
func bodyOnly(cs message.ContentStorable) (string, error) {
	bodyField, hasBody := cs.ContentFields()[message.ContentRoleBody]
	var refused []string
	body := ""
	for field, text := range cs.RawContent() {
		switch {
		case hasBody && field == bodyField:
			body = text
		case text != "":
			refused = append(refused, fmt.Sprintf("%q", field))
		}
	}
	if bs, ok := cs.(message.BinaryStorable); ok {
		for field, bin := range bs.BinaryFields() {
			if len(bin.Data) > 0 {
				refused = append(refused, fmt.Sprintf("binary %q", field))
			}
		}
	}
	if len(refused) > 0 {
		sort.Strings(refused)
		return "", fmt.Errorf("content other than the body is not stored (put non-body text in the graph as triples): %s",
			strings.Join(refused, ", "))
	}
	return body, nil
}

// contentKey is a new key for one StoreContent call:
// content/<yyyy>/<mm>/<dd>/<hh>/<entityID>_<uuid>. The clock only partitions;
// the UUID makes every call's key distinct.
func contentKey(entityID string) string {
	now := time.Now()
	return fmt.Sprintf("content/%04d/%02d/%02d/%02d/%s_%s",
		now.Year(), now.Month(), now.Day(), now.Hour(), entityID, uuid.NewString())
}
