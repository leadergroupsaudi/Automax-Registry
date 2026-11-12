package database

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestResourceGeoKeepsNewerFixUnderConcurrentWrites(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewResourceGeoStore(rdb)
	ctx := context.Background()
	departmentID := uuid.New()
	resourceID := uuid.New()
	base := time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fix := base.Add(time.Duration(i) * time.Second)
			lat := 12.0 + float64(i)/1000
			lng := 77.0 + float64(i)/1000
			if _, err := store.SavePosition(ctx, departmentID, resourceID, lat, lng, fix); err != nil {
				t.Errorf("save %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	fields, err := rdb.HGetAll(ctx, resourcePosKey(resourceID)).Result()
	if err != nil {
		t.Fatal(err)
	}
	ms, err := strconv.ParseInt(fields["updated_ms"], 10, 64)
	newest := base.Add(31 * time.Second).UnixMilli()
	if err != nil || ms != newest {
		t.Fatalf("updated_ms %q want %d err %v", fields["updated_ms"], newest, err)
	}
	pos, err := rdb.GeoPos(ctx, resourceGeoKey(departmentID), resourceID.String()).Result()
	if err != nil || len(pos) != 1 || pos[0] == nil {
		t.Fatalf("geopos %#v err %v", pos, err)
	}
	if !near(pos[0].Latitude, 12.031) || !near(pos[0].Longitude, 77.031) {
		t.Fatalf("newest point not kept lat=%v lng=%v", pos[0].Latitude, pos[0].Longitude)
	}

	older := base.Add(-time.Hour)
	wrote, err := store.SavePosition(ctx, departmentID, resourceID, 1, 2, older)
	if err != nil || wrote {
		t.Fatalf("older wrote %v err %v", wrote, err)
	}
	pos, err = rdb.GeoPos(ctx, resourceGeoKey(departmentID), resourceID.String()).Result()
	if err != nil || pos[0] == nil || !near(pos[0].Latitude, 12.031) {
		t.Fatalf("older fix replaced the point %#v err %v", pos, err)
	}
}

func TestResourceGeoMovesDepartmentAndClearsDuty(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	store := NewResourceGeoStore(rdb)
	ctx := context.Background()
	from := uuid.New()
	to := uuid.New()
	resourceID := uuid.New()
	operatorID := uuid.New()
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

	if _, err := store.SavePosition(ctx, from, resourceID, 12.97, 77.59, base); err != nil {
		t.Fatal(err)
	}
	if err := store.PutOperator(ctx, resourceID, &operatorID); err != nil {
		t.Fatal(err)
	}
	if err := store.PutResourceState(ctx, resourceID, "available", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SavePosition(ctx, to, resourceID, 13, 78, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	left, err := rdb.GeoPos(ctx, resourceGeoKey(from), resourceID.String()).Result()
	if err != nil || len(left) != 1 || left[0] != nil {
		t.Fatalf("old department still has the member %#v err %v", left, err)
	}
	gotOp, hit, err := store.Operator(ctx, resourceID)
	if err != nil || !hit || gotOp == nil || *gotOp != operatorID {
		t.Fatalf("operator %#v hit=%v err %v", gotOp, hit, err)
	}
	if err := store.PutOperator(ctx, resourceID, nil); err != nil {
		t.Fatal(err)
	}
	got, hit, err := store.Operator(ctx, resourceID)
	if err != nil || hit || got != nil {
		t.Fatalf("off duty hit=%v operator=%v err=%v", hit, got, err)
	}
	if err := store.Forget(ctx, resourceID); err != nil {
		t.Fatal(err)
	}
	left, err = rdb.GeoPos(ctx, resourceGeoKey(to), resourceID.String()).Result()
	if err != nil || len(left) != 1 || left[0] != nil {
		t.Fatalf("forget left the member %#v err %v", left, err)
	}
	_, hit, err = store.Operator(ctx, resourceID)
	if err != nil || hit {
		t.Fatalf("operator cache hit=%v err=%v", hit, err)
	}
}
