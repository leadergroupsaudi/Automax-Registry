package services

import (
	"context"
	"errors"
	"log"

	"github.com/automax/registry/internal/repository"
)

// thingCoordinateWriter updates things.latitude and things.longitude from a live fix.
// The device id is the external id stored on thing_external_refs.
type thingCoordinateWriter struct {
	system string
	refs   repository.ExternalRefRepository
	things repository.ThingRepository
}

func NewThingCoordinateWriter(system string, refs repository.ExternalRefRepository, things repository.ThingRepository) ThingCoordinateWriter {
	return &thingCoordinateWriter{system: system, refs: refs, things: things}
}

func (w *thingCoordinateWriter) Apply(ctx context.Context, deviceID string, latitude, longitude float64) error {
	if w == nil {
		return nil
	}
	linked, err := w.refs.FindLinkedThing(ctx, w.system, deviceID)
	if repository.IsNotFound(err) {
		log.Printf("registry: thing coordinates unchanged reason=no link device_id=%s system=%s", deviceID, w.system)
		return nil
	}
	if errors.Is(err, repository.ErrAmbiguousExternalRef) {
		log.Printf("registry: thing coordinates unchanged reason=ambiguous device_id=%s system=%s", deviceID, w.system)
		return nil
	}
	if err != nil {
		return err
	}
	if err := w.things.UpdateCoordinates(ctx, linked.ThingID, latitude, longitude); err != nil {
		return err
	}
	log.Printf("registry: thing coordinates updated thing_id=%s device_id=%s lat=%v lng=%v", linked.ThingID, deviceID, latitude, longitude)
	return nil
}
