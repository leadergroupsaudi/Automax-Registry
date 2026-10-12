package database

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestGeoStoreKeepsNewerFixOnly(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewGeoStore(rdb)
	const deviceID = "428976543"
	ctx := context.Background()

	newer := time.Date(2026, 10, 9, 8, 30, 0, 0, time.UTC)
	wrote, err := store.SaveLatest(ctx, deviceID, "Unit 1", 12.9716, 77.5946, newer)
	if err != nil || !wrote {
		t.Fatalf("wrote %v err %v", wrote, err)
	}
	older := newer.Add(-time.Hour)
	wrote, err = store.SaveLatest(ctx, deviceID, "Renamed", 1, 2, older)
	if err != nil || wrote {
		t.Fatalf("older wrote %v err %v", wrote, err)
	}

	pos, err := rdb.GeoPos(ctx, GeoDevicesKey, deviceID).Result()
	if err != nil || len(pos) != 1 || pos[0] == nil {
		t.Fatalf("geopos %#v err %v", pos, err)
	}
	if !near(pos[0].Longitude, 77.5946) || !near(pos[0].Latitude, 12.9716) {
		t.Fatalf("geo point lng=%v lat=%v", pos[0].Longitude, pos[0].Latitude)
	}
	fields, err := rdb.HGetAll(ctx, DevicePosPrefix+deviceID).Result()
	if err != nil {
		t.Fatal(err)
	}
	if fields["lng"] != "77.5946" || fields["lat"] != "12.9716" || fields["device_name"] != "Unit 1" {
		t.Fatalf("hash %#v", fields)
	}
	ms, err := strconv.ParseInt(fields["updated_ms"], 10, 64)
	if err != nil || ms != newer.UnixMilli() {
		t.Fatalf("updated_ms %q err %v", fields["updated_ms"], err)
	}
}

func near(got, want float64) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d < 0.00001
}
