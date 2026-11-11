package services

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

// resourcePositionStore is the Redis GEO write used by a live Caguar fix.
type resourcePositionStore interface {
	SavePosition(ctx context.Context, departmentID, resourceID uuid.UUID, latitude, longitude float64, fixTime time.Time) (bool, error)
}

// ResourceFixWriter copies a Caguar device fix onto the linked resource GEO set.
type deviceLinker interface {
	FindLinkedThing(ctx context.Context, system, externalID string) (repository.LinkedThing, error)
}

type ResourceFixWriter struct {
	system string
	refs   deviceLinker
	lookup repository.ResourceLookup
	geo    resourcePositionStore
}

func NewResourceFixWriter(system string, refs deviceLinker, lookup repository.ResourceLookup, geo resourcePositionStore) *ResourceFixWriter {
	return &ResourceFixWriter{system: system, refs: refs, lookup: lookup, geo: geo}
}

// ApplyFix GEOADDs resource:geo:<departmentID> when the device id is linked to a resource.
func (w *ResourceFixWriter) ApplyFix(ctx context.Context, deviceID string, latitude, longitude float64, fixTime time.Time) error {
	if w == nil || w.geo == nil || w.refs == nil || w.lookup == nil {
		return nil
	}
	linked, err := w.refs.FindLinkedThing(ctx, w.system, deviceID)
	if repository.IsNotFound(err) {
		log.Printf("registry: resource position unchanged reason=no link device_id=%s", deviceID)
		return nil
	}
	if errors.Is(err, repository.ErrAmbiguousExternalRef) {
		log.Printf("registry: resource position unchanged reason=ambiguous device_id=%s", deviceID)
		return nil
	}
	if err != nil {
		return err
	}
	target, err := w.lookup.FixTargetByThing(ctx, linked.ThingID)
	if err != nil {
		return err
	}
	if target == nil {
		log.Printf("registry: resource position unchanged reason=no resource thing_id=%s device_id=%s", linked.ThingID, deviceID)
		return nil
	}
	wrote, err := w.geo.SavePosition(ctx, target.DepartmentID, target.ResourceID, latitude, longitude, fixTime)
	if err != nil {
		return err
	}
	if !wrote {
		log.Printf("registry: resource position not stored reason=stale resource_id=%s device_id=%s", target.ResourceID, deviceID)
		return nil
	}
	log.Printf("registry: resource position stored resource_id=%s department_id=%s device_id=%s lat=%v lng=%v",
		target.ResourceID, target.DepartmentID, deviceID, latitude, longitude)
	return nil
}
