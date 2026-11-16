package services

import (
	"testing"

	"github.com/automax/registry/internal/caguar"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

func TestPairFollowsRenamedTrackerID(t *testing.T) {
	engine := uuid.New()
	engineRef := uuid.New()
	old := uuid.New()
	oldRef := uuid.New()
	links := []repository.NamedExternalRef{
		{RefID: oldRef, ThingID: old, ThingName: "Device MWBS45JM5532", ExternalID: "DEVMWBS45JM5532"},
		{RefID: engineRef, ThingID: engine, ThingName: "Engine 1122", ExternalID: "UNIT5548"},
	}
	devices := []caguarFix{{
		Name: "Engine 1122", UniqueID: "DEVMWBS45JM5532", Lat: 28.430542, Lng: 77.036889, HasFix: true,
	}}
	updates := pairCaguarDevices(links, devices)
	if len(updates) != 1 {
		t.Fatalf("updates %#v", updates)
	}
	got := updates[0]
	if got.ThingID != engine || got.ExternalID != "DEVMWBS45JM5532" || !got.ChangedID || !got.HasFix {
		t.Fatalf("update %#v", got)
	}
	if len(got.Drop) != 1 || got.Drop[0] != oldRef {
		t.Fatalf("drop %#v", got.Drop)
	}
	if got.Lat != 28.430542 || got.Lng != 77.036889 {
		t.Fatalf("coords %v %v", got.Lat, got.Lng)
	}
}

func TestPairKeepsMatchingUniqueID(t *testing.T) {
	thing := uuid.New()
	ref := uuid.New()
	links := []repository.NamedExternalRef{
		{RefID: ref, ThingID: thing, ThingName: "Engine 9", ExternalID: "UNIT9"},
	}
	devices := []caguarFix{{Name: "Engine 9", UniqueID: "UNIT9", Lat: 29.1, Lng: 48.1, HasFix: true}}
	updates := pairCaguarDevices(links, devices)
	if len(updates) != 1 || updates[0].ChangedID || updates[0].ThingID != thing {
		t.Fatalf("updates %#v", updates)
	}
}

func TestFixesSkipInvalidPositions(t *testing.T) {
	no := false
	fixes := fixesByDevice(
		[]caguar.Device{{ID: 7, Name: "Engine 9", UniqueID: "UNIT9"}},
		[]caguar.Position{{DeviceID: 7, Latitude: 1, Longitude: 2, Valid: &no}},
	)
	if len(fixes) != 1 || fixes[0].HasFix {
		t.Fatalf("fixes %#v", fixes)
	}
}
