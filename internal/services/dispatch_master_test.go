package services

import (
	"context"
	"testing"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
)

func TestResourceTypeValidation(t *testing.T) {
	dept := uuid.New()
	if _, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{}); errFields(t, err)["name"] == "" || errFields(t, err)["department_id"] == "" {
		t.Fatalf("missing %#v", err)
	}
	timeout := 0
	if _, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{DepartmentID: dept, Name: "Truck", AcceptanceTimeoutSeconds: &timeout}); errFields(t, err)["acceptance_timeout_seconds"] == "" {
		t.Fatal(err)
	}
	min, max := 80, 10
	if _, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{DepartmentID: dept, Name: "Truck", MinSpeed: &min, MaxSpeed: &max}); errFields(t, err)["max_speed"] == "" {
		t.Fatal(err)
	}
	negative := -1
	if _, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{DepartmentID: dept, Name: "Truck", MinWeight: &negative}); errFields(t, err)["min_weight"] == "" {
		t.Fatal(err)
	}
	bad := "red"
	if _, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{DepartmentID: dept, Name: "Truck", Colors: &models.AttributeColors{Available: bad}}); errFields(t, err)["colors.available"] == "" {
		t.Fatal(err)
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'A'
	}
	code := string(long)
	if _, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{DepartmentID: dept, Name: "Truck", Code: &code}); errFields(t, err)["code"] == "" {
		t.Fatal(err)
	}

	if _, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{DepartmentID: dept, Name: "●●●●"}); errFields(t, err)["name"] != "name must not contain special characters" {
		t.Fatalf("special name %#v", err)
	}
	arabic := "شاحنة"
	if _, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{DepartmentID: dept, Name: "Fire Truck", AltName: &arabic}); err != nil {
		t.Fatal(err)
	}

	row, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{DepartmentID: dept, Name: " Truck ", Code: strPtr("TRK")})
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != "Truck" || row.Code != "TRK" || row.DepartmentID != dept || !row.IsActive || row.AcceptanceTimeoutSeconds != 120 {
		t.Fatalf("row %#v", row)
	}
}

func TestServicePointValidation(t *testing.T) {
	dept := uuid.New()
	if _, err := mergeServicePoint(nil, models.ServicePointWriteRequest{}); errFields(t, err)["name"] == "" || errFields(t, err)["department_id"] == "" || errFields(t, err)["location"] == "" {
		t.Fatalf("missing %#v", err)
	}
	lat, lng := 29.1, 48.1
	radio := ""
	if _, err := mergeServicePoint(nil, models.ServicePointWriteRequest{
		Name: "Manga", DepartmentID: dept, RadioID: &radio, Location: &models.LocationInput{Lat: &lat, Lng: &lng},
	}); errFields(t, err)["radio_id"] == "" {
		t.Fatal(err)
	}
	badLat := 91.0
	radio = "FS-1"
	if _, err := mergeServicePoint(nil, models.ServicePointWriteRequest{
		Name: "Manga", DepartmentID: dept, RadioID: &radio, Location: &models.LocationInput{Lat: &badLat, Lng: &lng},
	}); errFields(t, err)["lat"] == "" {
		t.Fatal(err)
	}
	badLng := 181.0
	if _, err := mergeServicePoint(nil, models.ServicePointWriteRequest{
		Name: "Manga", DepartmentID: dept, RadioID: &radio, Location: &models.LocationInput{Lat: &lat, Lng: &badLng},
	}); errFields(t, err)["lng"] == "" {
		t.Fatal(err)
	}
	ids := []uuid.UUID{uuid.Nil}
	if _, err := mergeServicePoint(nil, models.ServicePointWriteRequest{
		Name: "Manga", DepartmentID: dept, RadioID: &radio, Location: &models.LocationInput{Lat: &lat, Lng: &lng}, ResourceIDs: &ids,
	}); errFields(t, err)["resource_ids"] == "" {
		t.Fatal(err)
	}

	if _, err := mergeServicePoint(nil, models.ServicePointWriteRequest{
		Name: "@@@", DepartmentID: dept, RadioID: &radio, Location: &models.LocationInput{Lat: &lat, Lng: &lng},
	}); errFields(t, err)["name"] != "name must not contain special characters" {
		t.Fatalf("special station %#v", err)
	}

	row, err := mergeServicePoint(nil, models.ServicePointWriteRequest{
		Name: " Manga ", DepartmentID: dept, RadioID: &radio, Location: &models.LocationInput{Lat: &lat, Lng: &lng},
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != "Manga" || row.RadioID != "FS-1" || row.Latitude != lat || row.Longitude != lng || !row.IsActive {
		t.Fatalf("row %#v", row)
	}
}

func TestUnitNameRejectsSpecialCharacters(t *testing.T) {
	s := &ResourceService{}
	name := "●●●●"
	_, err := s.ensureThing(context.Background(), &models.ResourceWriteRequest{Name: &name, ResourceTypeID: uuid.New()})
	if errFields(t, err)["name"] != "name must not contain special characters" {
		t.Fatalf("special unit %#v", err)
	}
	plate := "●●"
	err = applyResourceDetails(&models.Resource{}, nil, models.ResourceWriteRequest{PlateNumber: &plate})
	if errFields(t, err)["plate_number"] != "plate_number must not contain special characters" {
		t.Fatalf("special plate %#v", err)
	}
}

func strPtr(value string) *string { return &value }
