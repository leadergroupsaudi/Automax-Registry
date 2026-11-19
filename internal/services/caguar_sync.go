package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
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
	portal    caguarCatalog
	refs      repository.ExternalRefRepository
	things    repository.ThingRepository
	resources repository.ResourceRepository
	outbox    repository.CaguarSyncRepository
	writer    Portal
	every     time.Duration

	mu   sync.Mutex
	last time.Time
}

func NewCaguarSync(portal caguarCatalog, refs repository.ExternalRefRepository, things repository.ThingRepository) *CaguarSync {
	if portal == nil {
		return nil
	}
	return &CaguarSync{portal: portal, refs: refs, things: things, every: 3 * time.Second}
}

func (s *CaguarSync) UseResources(resources repository.ResourceRepository) {
	if s == nil {
		return
	}
	s.resources = resources
}

func (s *CaguarSync) UseOutbox(outbox repository.CaguarSyncRepository) {
	if s == nil {
		return
	}
	s.outbox = outbox
}

func (s *CaguarSync) UseWriter(writer Portal) {
	if s == nil {
		return
	}
	s.writer = writer
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
	if err := s.flush(ctx); err != nil {
		log.Printf("registry: caguar device sync failed err=%v", err)
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
		if update.HasFix {
			if err := s.things.UpdateCoordinates(ctx, update.ThingID, update.Lat, update.Lng); err != nil {
				return err
			}
			log.Printf("registry: caguar position applied thing_id=%s unique_id=%s lat=%v lng=%v", update.ThingID, update.ExternalID, update.Lat, update.Lng)
		}
		if err := s.mirrorDevice(ctx, update); err != nil {
			log.Printf("registry: caguar field sync failed thing_id=%s unique_id=%s err=%v", update.ThingID, update.ExternalID, err)
		}
	}
	return nil
}

type caguarFix struct {
	Name     string
	UniqueID string
	Plate    string
	Chassis  string
	Tracker  string
	Product  string
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
	Name       string
	Plate      string
	Chassis    string
	Tracker    string
	Product    string
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
		fix := caguarFix{
			Name: name, UniqueID: uniqueID, Plate: strings.TrimSpace(device.PlateNumber),
			Chassis: strings.TrimSpace(device.ChassisNumber), Tracker: strings.TrimSpace(device.TrackerType),
			Product: strings.TrimSpace(device.Product),
		}
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
			Name:       device.Name,
			Plate:      device.Plate,
			Chassis:    device.Chassis,
			Tracker:    device.Tracker,
			Product:    device.Product,
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

func (s *CaguarSync) flush(ctx context.Context) error {
	if s == nil || s.outbox == nil || s.writer == nil {
		return nil
	}
	jobs, err := s.outbox.List(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		err := s.applyJob(ctx, job)
		if err == nil {
			if delErr := s.outbox.Delete(ctx, job.ID); delErr != nil {
				log.Printf("registry: caguar sync job done but not cleared id=%s action=%s err=%v", job.ID, job.Action, delErr)
			} else {
				log.Printf("registry: caguar sync job applied id=%s action=%s thing_id=%s unique_id=%s", job.ID, job.Action, job.ThingID, job.UniqueID)
			}
			continue
		}
		if caguarDown(err) {
			log.Printf("registry: caguar sync paused action=%s thing_id=%s unique_id=%s err=%v", job.Action, job.ThingID, job.UniqueID, err)
			return nil
		}
		_ = s.outbox.MarkError(ctx, job.ID, err.Error())
		log.Printf("registry: caguar sync job rejected action=%s thing_id=%s unique_id=%s err=%v", job.Action, job.ThingID, job.UniqueID, err)
	}
	return nil
}

func (s *CaguarSync) applyJob(ctx context.Context, job models.CaguarSyncJob) error {
	var reg DeviceRegistration
	if len(job.Payload) > 0 {
		_ = json.Unmarshal(job.Payload, &reg)
	}
	if reg.UniqueID == "" {
		reg.UniqueID = job.UniqueID
	}
	device := caguar.Device{
		Name:          reg.Name,
		UniqueID:      reg.UniqueID,
		Category:      "car",
		TrackerType:   reg.TrackerType,
		PlateNumber:   strings.ToUpper(strings.TrimSpace(reg.PlateNumber)),
		ChassisNumber: reg.ChassisNumber,
		ZoneIDs:       []int{},
		Attributes:    map[string]any{},
		Product:       caguar.ProductAutomax,
	}
	switch job.Action {
	case models.CaguarSyncDelete:
		err := s.writer.DeleteDevice(ctx, device.UniqueID)
		if err != nil && !caguarMissing(err) {
			return err
		}
		s.dropRef(ctx, job.ThingID)
		return nil
	case models.CaguarSyncUpdate:
		if err := s.writer.UpdateDevice(ctx, device.UniqueID, device); err != nil {
			if caguarMissing(err) {
				return s.writer.RegisterDevice(ctx, device)
			}
			return err
		}
		return nil
	default:
		if err := s.writer.RegisterDevice(ctx, device); err != nil {
			return err
		}
		return s.ensureRef(ctx, job.ThingID, device.UniqueID)
	}
}

func (s *CaguarSync) ensureRef(ctx context.Context, thingID uuid.UUID, uniqueID string) error {
	if s.refs == nil || thingID == uuid.Nil || uniqueID == "" {
		return nil
	}
	ref, err := s.refs.FindByThingSystem(ctx, thingID, models.ExternalSystemTraccar)
	if repository.IsNotFound(err) {
		return s.refs.Create(ctx, &models.ThingExternalRef{
			ThingID:        thingID,
			ExternalSystem: models.ExternalSystemTraccar,
			ExternalID:     uniqueID,
		})
	}
	if err != nil {
		return err
	}
	if ref.ExternalID == uniqueID {
		return nil
	}
	return s.refs.UpdateExternalID(ctx, ref.ID, uniqueID, time.Now().UTC())
}

func (s *CaguarSync) dropRef(ctx context.Context, thingID uuid.UUID) {
	if s.refs == nil || thingID == uuid.Nil {
		return
	}
	ref, err := s.refs.FindByThingSystem(ctx, thingID, models.ExternalSystemTraccar)
	if err != nil {
		return
	}
	if err := s.refs.Delete(ctx, ref.ID); err != nil && !repository.IsNotFound(err) {
		log.Printf("registry: caguar sync ref delete failed thing_id=%s ref_id=%s err=%v", thingID, ref.ID, err)
	}
}

func (s *CaguarSync) mirrorDevice(ctx context.Context, update caguarUpdate) error {
	if s == nil || s.things == nil || s.resources == nil {
		return nil
	}
	if s.outbox != nil {
		jobs, err := s.outbox.List(ctx)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			if job.ThingID == update.ThingID && job.Action != models.CaguarSyncDelete {
				return nil
			}
		}
	}
	thing, err := s.things.FindByID(ctx, update.ThingID)
	if err != nil {
		return err
	}
	meta := map[string]string{}
	if len(thing.Metadata) > 0 {
		_ = json.Unmarshal(thing.Metadata, &meta)
	}
	changed := false
	if update.Name != "" && thing.Name != update.Name {
		thing.Name = update.Name
		changed = true
	}
	if update.Chassis != "" && meta["chassis_number"] != update.Chassis {
		meta["chassis_number"] = update.Chassis
		changed = true
	}
	if update.Tracker != "" && meta["tracker_type"] != update.Tracker {
		meta["tracker_type"] = update.Tracker
		changed = true
	}
	if update.ExternalID != "" && meta["unique_id"] != update.ExternalID {
		meta["unique_id"] = update.ExternalID
		changed = true
	}
	if changed {
		raw, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		thing.Metadata = models.JSON(raw)
		if err := s.things.Update(ctx, thing); err != nil {
			return err
		}
		log.Printf("registry: caguar fields applied thing_id=%s name=%s unique_id=%s", thing.ID, thing.Name, update.ExternalID)
	}
	if update.Plate == "" {
		return nil
	}
	row, err := s.resources.FindByThingID(ctx, update.ThingID)
	if repository.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.PlateNumber == update.Plate {
		return nil
	}
	if err := s.resources.UpdatePlate(ctx, row.ID, update.Plate); err != nil {
		return err
	}
	log.Printf("registry: caguar plate applied thing_id=%s resource_id=%s plate=%s", update.ThingID, row.ID, update.Plate)
	return nil
}

func caguarDown(err error) bool {
	if err == nil {
		return false
	}
	var remote *caguar.RemoteError
	if errors.As(err, &remote) {
		return remote.Status >= http.StatusInternalServerError
	}
	return caguarDeferred(err) || !errors.As(err, &remote)
}

func caguarMissing(err error) bool {
	var remote *caguar.RemoteError
	return errors.As(err, &remote) && remote.Status == http.StatusNotFound
}
