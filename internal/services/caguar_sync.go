package services

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/automax/registry/internal/caguar"
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

// caguarCatalog is the slice of the portal the location sync reads.
type caguarCatalog interface {
	Devices(ctx context.Context) ([]caguar.Device, error)
	Positions(ctx context.Context) ([]caguar.Position, error)
}

// CaguarSync copies the latest Caguar fix onto the registry unit.
// A device created from a unit starts with the registry's id. When that id is
// replaced in Caguar with the phone's tracker id, the match follows the unit name.
type CaguarSync struct {
	portal caguarCatalog
	refs   repository.ExternalRefRepository
	things repository.ThingRepository
	every  time.Duration

	mu   sync.Mutex
	last time.Time
}

func NewCaguarSync(portal caguarCatalog, refs repository.ExternalRefRepository, things repository.ThingRepository) *CaguarSync {
	if portal == nil {
		return nil
	}
	return &CaguarSync{portal: portal, refs: refs, things: things, every: 3 * time.Second}
}

// Refresh pulls Caguar when the last pull is older than a few seconds.
// A portal error leaves the unit list unchanged.
func (s *CaguarSync) Refresh(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("registry: caguar position sync failed panic=%v", rec)
		}
	}()
	if s == nil || s.portal == nil || s.refs == nil || s.things == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.last.IsZero() && time.Since(s.last) < s.every {
		return
	}
	if err := s.pull(ctx); err != nil {
		log.Printf("registry: caguar position sync failed err=%v", err)
	}
	s.last = time.Now()
}

func (s *CaguarSync) pull(ctx context.Context) error {
	devices, err := s.portal.Devices(ctx)
	if err != nil {
		return err
	}
	positions, err := s.portal.Positions(ctx)
	if err != nil {
		return err
	}
	links, err := s.refs.ListNamed(ctx, models.ExternalSystemTraccar)
	if err != nil {
		return err
	}
	fixes := fixesByDevice(devices, positions)
	for _, update := range pairCaguarDevices(links, fixes) {
		for _, drop := range update.Drop {
			if err := s.refs.Delete(ctx, drop); err != nil && !repository.IsNotFound(err) {
				return err
			}
			log.Printf("registry: caguar device id released ref_id=%s", drop)
		}
		if update.ChangedID {
			if err := s.refs.UpdateExternalID(ctx, update.RefID, update.ExternalID, time.Now().UTC()); err != nil {
				return err
			}
			log.Printf("registry: caguar device id updated thing_id=%s unique_id=%s", update.ThingID, update.ExternalID)
		}
		if !update.HasFix {
			continue
		}
		if err := s.things.UpdateCoordinates(ctx, update.ThingID, update.Lat, update.Lng); err != nil {
			return err
		}
		log.Printf("registry: caguar position applied thing_id=%s unique_id=%s lat=%v lng=%v", update.ThingID, update.ExternalID, update.Lat, update.Lng)
	}
	return nil
}

type caguarFix struct {
	Name     string
	UniqueID string
	Lat      float64
	Lng      float64
	HasFix   bool
}

type caguarUpdate struct {
	RefID      uuid.UUID
	ThingID    uuid.UUID
	ExternalID string
	ChangedID  bool
	Drop       []uuid.UUID
	Lat        float64
	Lng        float64
	HasFix     bool
}

func fixesByDevice(devices []caguar.Device, positions []caguar.Position) []caguarFix {
	point := map[int]caguar.Position{}
	for _, position := range positions {
		if position.Valid != nil && !*position.Valid {
			continue
		}
		point[position.DeviceID] = position
	}
	out := make([]caguarFix, 0, len(devices))
	for _, device := range devices {
		uniqueID := strings.TrimSpace(device.UniqueID)
		name := strings.TrimSpace(device.Name)
		if device.ID == 0 || uniqueID == "" || name == "" {
			continue
		}
		fix := caguarFix{Name: name, UniqueID: uniqueID}
		if position, ok := point[device.ID]; ok {
			fix.Lat = position.Latitude
			fix.Lng = position.Longitude
			fix.HasFix = true
		}
		out = append(out, fix)
	}
	return out
}

// pairCaguarDevices chooses which registry thing receives each Caguar device.
// The phone's tracker id wins when it was pasted onto the device created for that unit.
func pairCaguarDevices(links []repository.NamedExternalRef, devices []caguarFix) []caguarUpdate {
	byUnique := map[string]caguarFix{}
	byName := map[string][]caguarFix{}
	for _, device := range devices {
		byUnique[device.UniqueID] = device
		byName[fold(device.Name)] = append(byName[fold(device.Name)], device)
	}
	holder := map[string]uuid.UUID{}
	for _, link := range links {
		if link.ExternalID != "" {
			holder[link.ExternalID] = link.RefID
		}
	}

	claimed := map[string]uuid.UUID{}
	chosen := map[uuid.UUID]caguarFix{}
	claim := func(link repository.NamedExternalRef, device caguarFix) {
		if _, taken := claimed[device.UniqueID]; taken {
			return
		}
		claimed[device.UniqueID] = link.ThingID
		chosen[link.RefID] = device
	}

	for _, link := range links {
		device, ok := byUnique[link.ExternalID]
		if ok && fold(device.Name) == fold(link.ThingName) {
			claim(link, device)
		}
	}
	for _, link := range links {
		if _, ok := chosen[link.RefID]; ok {
			continue
		}
		if _, stillThere := byUnique[link.ExternalID]; stillThere {
			continue
		}
		named := byName[fold(link.ThingName)]
		if len(named) != 1 {
			continue
		}
		claim(link, named[0])
	}
	for _, link := range links {
		if _, ok := chosen[link.RefID]; ok {
			continue
		}
		device, ok := byUnique[link.ExternalID]
		if !ok {
			continue
		}
		claim(link, device)
	}

	updates := make([]caguarUpdate, 0, len(chosen))
	for _, link := range links {
		device, ok := chosen[link.RefID]
		if !ok {
			continue
		}
		update := caguarUpdate{
			RefID:      link.RefID,
			ThingID:    link.ThingID,
			ExternalID: device.UniqueID,
			ChangedID:  device.UniqueID != link.ExternalID,
			Lat:        device.Lat,
			Lng:        device.Lng,
			HasFix:     device.HasFix,
		}
		if update.ChangedID {
			if other, used := holder[device.UniqueID]; used && other != link.RefID {
				update.Drop = []uuid.UUID{other}
			}
		}
		updates = append(updates, update)
	}
	return updates
}

func fold(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
