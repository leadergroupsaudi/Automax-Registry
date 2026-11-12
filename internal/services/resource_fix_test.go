package services

import (
	"context"
	"testing"
	"time"

	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestLiveFixUpdatesResourceGeo(t *testing.T) {
	thingID := uuid.New()
	resourceID := uuid.New()
	dept := uuid.New()
	store := newMemGeo()
	writer := NewResourceFixWriter("traccar-go", linkStub{thingID: thingID}, &fakeLookup{
		target: &repository.ResourceFixTarget{ResourceID: resourceID, DepartmentID: dept},
	}, store)
	when := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	if err := writer.ApplyFix(context.Background(), "TESTGO123", 12.972, 77.5946, when); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	got := store.pos[resourceID]
	store.mu.Unlock()
	if got.dept != dept || got.lat != 12.972 || got.lng != 77.5946 {
		t.Fatalf("stored %#v", got)
	}
	if err := writer.ApplyFix(context.Background(), "TESTGO123", 1, 2, when.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	got = store.pos[resourceID]
	store.mu.Unlock()
	if got.lat != 12.972 {
		t.Fatalf("older fix replaced %#v", got)
	}
}

type linkStub struct{ thingID uuid.UUID }

func (s linkStub) FindLinkedThing(context.Context, string, string) (repository.LinkedThing, error) {
	if s.thingID == uuid.Nil {
		return repository.LinkedThing{}, gorm.ErrRecordNotFound
	}
	return repository.LinkedThing{ThingID: s.thingID, IsTrackable: true}, nil
}
