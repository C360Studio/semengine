package clustering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/types"
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

// get reads the record at key through DecodeCommunity.
func (s *NATSCommunityStorage) get(ctx context.Context, key string) (*Community, error) {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, errs.WrapTransient(fmt.Errorf("read %s: %w", key, err), "NATSCommunityStorage", "GetAllCommunities", "read community")
	}
	return DecodeCommunity(key, entry.Value)
}

// keys lists every key in the bucket, sorted.
func (s *NATSCommunityStorage) keys(ctx context.Context) ([]string, error) {
	keys, err := s.kv.Keys(ctx)
	if err != nil {
		return nil, errs.WrapTransient(err, "NATSCommunityStorage", "keys", "list keys")
	}
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

// GroupKey returns the COMMUNITY_INDEX key of entityID's group at level: the level, a
// dot, and the ID's first four (level 0), three (1) or two (2) positions. It refuses, with
// an invalid error, a level outside 0 to 2 and an ID the canonical parser refuses.
func GroupKey(entityID string, level int) (string, error) {
	prefix, err := LevelPrefix(level)
	if err != nil {
		return "", err
	}
	eid, err := types.ParseEntityID(entityID)
	if err != nil {
		return "", errs.WrapInvalid(fmt.Errorf("entity %q: %w", entityID, err), "clustering", "GroupKey", "parse entity ID")
	}
	return prefix + groupIDs(eid)[level], nil
}

// LevelPrefix returns the prefix every COMMUNITY_INDEX key at level starts with: the
// level and a dot. It refuses, with an invalid error, a level outside 0 to 2.
func LevelPrefix(level int) (string, error) {
	if level < 0 || level >= levels {
		return "", errs.WrapInvalid(fmt.Errorf("level %d is not 0, 1 or 2", level), "clustering", "LevelPrefix", "check level")
	}
	return levelPrefix(level), nil
}

// levelPrefix is LevelPrefix for a level the caller knows is 0 to 2, such as a record's
// own or an index into a [levels] array.
func levelPrefix(level int) string {
	return strconv.Itoa(level) + "."
}

// DecodeCommunity decodes value, the COMMUNITY_INDEX entry at key, as storage does. It
// refuses, with an invalid error naming key, a value that is not JSON, a record whose
// level is not 0, 1 or 2, a record whose level and ID do not give key, and a record whose
// EntityRevision is 0, which no maintainer writes.
func DecodeCommunity(key string, value []byte) (*Community, error) {
	refuse := func(err error) (*Community, error) {
		return nil, errs.WrapInvalid(err, "clustering", "DecodeCommunity", "decode community")
	}
	var community Community
	if err := json.Unmarshal(value, &community); err != nil {
		return refuse(fmt.Errorf("decode %s: %w", key, err))
	}
	prefix, err := LevelPrefix(community.Level)
	if err != nil {
		return refuse(fmt.Errorf("record at %s: %w", key, err))
	}
	if own := prefix + community.ID; own != key {
		return refuse(fmt.Errorf("%s holds the record of %q", key, own))
	}
	if community.EntityRevision == 0 {
		return refuse(fmt.Errorf("%s holds a record with entity_revision 0", key))
	}
	return &community, nil
}

// communityKey is a group record's key, {level}.{group ID} (pin storage.go:518-520).
// The caller knows level is 0 to 2.
func communityKey(level int, id string) string {
	return levelPrefix(level) + id
}
