package clustering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// NATSCommunityStorage reads and writes group records in the COMMUNITY_INDEX bucket
// (graph.BucketCommunityIndex), one key per group: {level}.{group ID}.
type NATSCommunityStorage struct {
	kv *natsclient.KVStore
}

// NewNATSCommunityStorage returns a storage over kv, the COMMUNITY_INDEX bucket. It
// refuses a nil kv.
func NewNATSCommunityStorage(kv *natsclient.KVStore) (*NATSCommunityStorage, error) {
	if kv == nil {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "NATSCommunityStorage", "NewNATSCommunityStorage", "kv store is nil")
	}
	return &NATSCommunityStorage{kv: kv}, nil
}

// GetAllCommunities returns every record in the bucket, in key order. An entry that
// cannot be read, or is not the record of the group its key names, fails the call with
// an error naming the key; an ended context returns its error. An empty bucket gives no
// records and no error.
func (s *NATSCommunityStorage) GetAllCommunities(ctx context.Context) ([]*Community, error) {
	if ctx == nil {
		return nil, errs.WrapInvalid(errors.New("nil context"), "NATSCommunityStorage", "GetAllCommunities", "missing context")
	}
	keys, err := s.keys(ctx)
	if err != nil {
		return nil, err
	}
	communities := make([]*Community, 0, len(keys))
	for _, key := range keys {
		community, err := s.get(ctx, key)
		if err != nil {
			return nil, err
		}
		communities = append(communities, community)
	}
	return communities, nil
}

// get reads and decodes the record at key, refusing a record whose own level and ID do
// not give key back.
func (s *NATSCommunityStorage) get(ctx context.Context, key string) (*Community, error) {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, errs.WrapTransient(fmt.Errorf("read %s: %w", key, err), "NATSCommunityStorage", "GetAllCommunities", "read community")
	}
	var community Community
	if err := json.Unmarshal(entry.Value, &community); err != nil {
		return nil, errs.WrapInvalid(fmt.Errorf("decode %s: %w", key, err), "NATSCommunityStorage", "GetAllCommunities", "decode community")
	}
	if own := communityKey(community.Level, community.ID); own != key {
		return nil, errs.WrapInvalid(fmt.Errorf("%s holds the record of %q", key, own), "NATSCommunityStorage", "GetAllCommunities", "decode community")
	}
	return &community, nil
}

// keys lists every key in the bucket, sorted.
func (s *NATSCommunityStorage) keys(ctx context.Context) ([]string, error) {
	keys, err := s.kv.Keys(ctx)
	if err != nil {
		return nil, errs.WrapTransient(err, "NATSCommunityStorage", "keys", "list keys")
	}
	slices.Sort(keys)
	return keys, nil
}

// save writes community at its key, replacing what was there.
func (s *NATSCommunityStorage) save(ctx context.Context, community *Community) error {
	key := communityKey(community.Level, community.ID)
	if err := natsclient.ValidateKVLiteralKey(key); err != nil {
		return errs.WrapInvalid(err, "NATSCommunityStorage", "save", "validate key")
	}
	data, err := json.Marshal(community)
	if err != nil {
		return errs.WrapInvalid(err, "NATSCommunityStorage", "save", "encode community")
	}
	if _, err := s.kv.Put(ctx, key, data); err != nil {
		return errs.WrapTransient(err, "NATSCommunityStorage", "save", "put community")
	}
	return nil
}

// remove deletes the record at key. A key that is not there is already removed.
func (s *NATSCommunityStorage) remove(ctx context.Context, key string) error {
	if err := natsclient.ValidateKVLiteralKey(key); err != nil {
		return errs.WrapInvalid(err, "NATSCommunityStorage", "remove", "validate key")
	}
	if err := s.kv.Delete(ctx, key); err != nil && !errors.Is(err, natsclient.ErrKVKeyNotFound) {
		return errs.WrapTransient(err, "NATSCommunityStorage", "remove", "delete community")
	}
	return nil
}

// communityKey is a group record's key, {level}.{group ID} (pin storage.go:518-520).
func communityKey(level int, id string) string {
	return strconv.Itoa(level) + "." + id
}
