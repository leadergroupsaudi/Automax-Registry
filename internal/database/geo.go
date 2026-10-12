package database

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// GeoDevicesKey is the Redis GEO set of latest device positions.
	// The member is the Caugar device id. Longitude is stored first.
	GeoDevicesKey = "geo:devices"
	// DevicePosPrefix is the hash that holds freshness for one device: device:pos:{deviceId}.
	DevicePosPrefix = "device:pos:"
)

// saveLatestPosition writes a fix only when its timestamp is newer than the stored one.
// KEYS[1] hash, KEYS[2] geo set.
// ARGV: updated_ms, longitude, latitude, member, updated_at, device_name.
var saveLatestPosition = redis.NewScript(`
local prev = redis.call('HGET', KEYS[1], 'updated_ms')
if prev and tonumber(prev) >= tonumber(ARGV[1]) then
  return 0
end
redis.call('GEOADD', KEYS[2], ARGV[2], ARGV[3], ARGV[4])
redis.call('HSET', KEYS[1], 'lng', ARGV[2], 'lat', ARGV[3], 'updated_at', ARGV[5], 'updated_ms', ARGV[1])
if ARGV[6] ~= '' then
  redis.call('HSET', KEYS[1], 'device_name', ARGV[6])
end
return 1
`)

// GeoStore keeps the latest position for each Caugar device id.
type GeoStore struct {
	client *redis.Client
}

func NewGeoStore(client *redis.Client) *GeoStore {
	return &GeoStore{client: client}
}

func devicePosKey(deviceID string) string {
	return DevicePosPrefix + deviceID
}

// SaveLatest stores the position when fixTime is newer than the current point.
// wrote is false when this fix is older or the same time as the stored one.
func (s *GeoStore) SaveLatest(ctx context.Context, deviceID, deviceName string, latitude, longitude float64, fixTime time.Time) (bool, error) {
	fixTime = fixTime.UTC()
	n, err := saveLatestPosition.Run(ctx, s.client, []string{
		devicePosKey(deviceID),
		GeoDevicesKey,
	},
		strconv.FormatInt(fixTime.UnixMilli(), 10),
		strconv.FormatFloat(longitude, 'f', -1, 64),
		strconv.FormatFloat(latitude, 'f', -1, 64),
		deviceID,
		fixTime.Format(time.RFC3339Nano),
		deviceName,
	).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
