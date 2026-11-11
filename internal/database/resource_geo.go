package database

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	resourceGeoPrefix      = "resource:geo:"
	resourcePosPrefix      = "resource:pos:"
	resourceOperatorPrefix = "resource:operator:"
	resourceStatusPrefix   = "resource:status:"
	resourceTypePrefix     = "resource:type:"

	// operatorNone is a cached "off duty" so a miss can be told apart from a known empty shift.
	operatorNone = "none"
)

// saveResourcePosition writes a fix only when its timestamp is newer than the stored one.
// A department change removes the member from the previous GEO set in the same script,
// so two concurrent telemetry messages cannot leave the resource in both sets or rewind it.
// KEYS[1] position hash, KEYS[2] department GEO set.
// ARGV: updated_ms, longitude, latitude, member, department_id.
var saveResourcePosition = redis.NewScript(`
local prev = redis.call('HGET', KEYS[1], 'updated_ms')
if prev and tonumber(prev) >= tonumber(ARGV[1]) then
  return 0
end
local oldDept = redis.call('HGET', KEYS[1], 'department_id')
if oldDept and oldDept ~= '' and oldDept ~= ARGV[5] then
  redis.call('ZREM', 'resource:geo:' .. oldDept, ARGV[4])
end
redis.call('GEOADD', KEYS[2], ARGV[2], ARGV[3], ARGV[4])
redis.call('HSET', KEYS[1], 'lng', ARGV[2], 'lat', ARGV[3], 'updated_ms', ARGV[1], 'department_id', ARGV[5])
return 1
`)

// forgetResourcePosition drops the GEO member and the cached duty and status keys.
// KEYS: position hash, operator, status, type. ARGV[1] is the resource id.
var forgetResourcePosition = redis.NewScript(`
local dept = redis.call('HGET', KEYS[1], 'department_id')
if dept and dept ~= '' then
  redis.call('ZREM', 'resource:geo:' .. dept, ARGV[1])
end
redis.call('DEL', KEYS[1], KEYS[2], KEYS[3], KEYS[4])
return 1
`)

// ResourceGeoStore is the Redis hot path for resource positions, duty, and status.
type ResourceGeoStore struct {
	client *redis.Client
}

func NewResourceGeoStore(client *redis.Client) *ResourceGeoStore {
	return &ResourceGeoStore{client: client}
}

func resourceGeoKey(departmentID uuid.UUID) string {
	return resourceGeoPrefix + departmentID.String()
}

func resourcePosKey(resourceID uuid.UUID) string {
	return resourcePosPrefix + resourceID.String()
}

func resourceOperatorKey(resourceID uuid.UUID) string {
	return resourceOperatorPrefix + resourceID.String()
}

func resourceStatusKey(resourceID uuid.UUID) string {
	return resourceStatusPrefix + resourceID.String()
}

func resourceTypeKey(resourceID uuid.UUID) string {
	return resourceTypePrefix + resourceID.String()
}

// SavePosition GEOADDs resource:geo:<departmentID> when fixTime is newer than the stored point.
// wrote is false when this fix is older or the same time as the stored one.
func (s *ResourceGeoStore) SavePosition(ctx context.Context, departmentID, resourceID uuid.UUID, latitude, longitude float64, fixTime time.Time) (bool, error) {
	if s == nil || s.client == nil {
		return false, errors.New("resource geo store is not configured")
	}
	fixTime = fixTime.UTC()
	n, err := saveResourcePosition.Run(ctx, s.client, []string{
		resourcePosKey(resourceID),
		resourceGeoKey(departmentID),
	},
		strconv.FormatInt(fixTime.UnixMilli(), 10),
		strconv.FormatFloat(longitude, 'f', -1, 64),
		strconv.FormatFloat(latitude, 'f', -1, 64),
		resourceID.String(),
		departmentID.String(),
	).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// Search returns every resource in the department GEO set inside radiusMeters, nearest first.
func (s *ResourceGeoStore) Search(ctx context.Context, departmentID uuid.UUID, latitude, longitude, radiusMeters float64) ([]models.GeoCandidate, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("resource geo store is not configured")
	}
	locs, err := s.client.GeoSearchLocation(ctx, resourceGeoKey(departmentID), &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  longitude,
			Latitude:   latitude,
			Radius:     radiusMeters,
			RadiusUnit: "m",
			Sort:       "ASC",
		},
		WithDist: true,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make([]models.GeoCandidate, 0, len(locs))
	for _, loc := range locs {
		id, err := uuid.Parse(loc.Name)
		if err != nil {
			continue
		}
		out = append(out, models.GeoCandidate{ResourceID: id, DistanceM: loc.Dist})
	}
	return out, nil
}

// Operator reads resource:operator:<resourceID>.
// hit is false on a cache miss. A hit with a nil operator means the resource is known to be off duty.
func (s *ResourceGeoStore) Operator(ctx context.Context, resourceID uuid.UUID) (*uuid.UUID, bool, error) {
	if s == nil || s.client == nil {
		return nil, false, errors.New("resource geo store is not configured")
	}
	val, err := s.client.Get(ctx, resourceOperatorKey(resourceID)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if val == operatorNone {
		return nil, true, nil
	}
	id, err := uuid.Parse(val)
	if err != nil {
		return nil, false, nil
	}
	return &id, true, nil
}

// PutOperator writes resource:operator:<resourceID> for an on-duty user.
// A nil operator removes the key, which is how a shift end is cached.
func (s *ResourceGeoStore) PutOperator(ctx context.Context, resourceID uuid.UUID, operatorID *uuid.UUID) error {
	if s == nil || s.client == nil {
		return errors.New("resource geo store is not configured")
	}
	if operatorID == nil || *operatorID == uuid.Nil {
		return s.client.Del(ctx, resourceOperatorKey(resourceID)).Err()
	}
	return s.client.Set(ctx, resourceOperatorKey(resourceID), operatorID.String(), 0).Err()
}

// ResourceState reads the cached DSP-01 status and resource type.
// hit is false when either value is missing.
func (s *ResourceGeoStore) ResourceState(ctx context.Context, resourceID uuid.UUID) (string, uuid.UUID, bool, error) {
	if s == nil || s.client == nil {
		return "", uuid.Nil, false, errors.New("resource geo store is not configured")
	}
	vals, err := s.client.MGet(ctx, resourceStatusKey(resourceID), resourceTypeKey(resourceID)).Result()
	if err != nil {
		return "", uuid.Nil, false, err
	}
	if len(vals) != 2 || vals[0] == nil || vals[1] == nil {
		return "", uuid.Nil, false, nil
	}
	status, _ := vals[0].(string)
	typeRaw, _ := vals[1].(string)
	typeID, err := uuid.Parse(typeRaw)
	if err != nil || status == "" {
		return "", uuid.Nil, false, nil
	}
	return status, typeID, true, nil
}

// PutResourceState caches status and resource type together.
func (s *ResourceGeoStore) PutResourceState(ctx context.Context, resourceID uuid.UUID, status string, resourceTypeID uuid.UUID) error {
	if s == nil || s.client == nil {
		return errors.New("resource geo store is not configured")
	}
	_, err := s.client.Pipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, resourceStatusKey(resourceID), status, 0)
		p.Set(ctx, resourceTypeKey(resourceID), resourceTypeID.String(), 0)
		return nil
	})
	return err
}

// Forget removes the GEO member and the operator, status, and type cache for one resource.
func (s *ResourceGeoStore) Forget(ctx context.Context, resourceID uuid.UUID) error {
	if s == nil || s.client == nil {
		return errors.New("resource geo store is not configured")
	}
	return forgetResourcePosition.Run(ctx, s.client, []string{
		resourcePosKey(resourceID),
		resourceOperatorKey(resourceID),
		resourceStatusKey(resourceID),
		resourceTypeKey(resourceID),
	}, resourceID.String()).Err()
}
