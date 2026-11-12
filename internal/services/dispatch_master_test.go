package services

import (
	"testing"
	"time"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
)

func TestMergeResourceTypeKeepsOmittedFields(t *testing.T) {
	till := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	dept := uuid.New()
	current := &models.ResourceType{
		DepartmentID: dept, Name: "Truck", Code: "TRK", AltName: "شاحنة", MinSpeed: 1, MaxSpeed: 80,
		IsActive: true, ActiveTillDate: &till, Icon: "truck.svg",
		Colors: mustColors(models.DefaultAttributeColors()),
	}
	next, err := mergeResourceType(current, models.ResourceTypeWriteRequest{
		DepartmentID: dept,
		Name:         " Engine ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Name != "Engine" || next.Code != "TRK" || next.AltName != "شاحنة" || next.MaxSpeed != 80 || next.Icon != "truck.svg" {
		t.Fatalf("merged %#v", next)
	}
	if next.ActiveTillDate == nil || !next.ActiveTillDate.Equal(till) || !next.IsActive {
		t.Fatalf("flags %#v active %v", next.ActiveTillDate, next.IsActive)
	}
}

func TestMergeResourceTypeRejectsBadColorAndSpeed(t *testing.T) {
	_, err := mergeResourceType(nil, models.ResourceTypeWriteRequest{
		Name: "Truck", MinSpeed: intPtr(10), MaxSpeed: intPtr(2),
		Colors: &models.AttributeColors{Available: "green"},
	})
	fields := errFields(t, err)
	if fields["department_id"] == "" || fields["max_speed"] == "" || fields["colors.available"] == "" {
		t.Fatalf("fields %#v", fields)
	}
}

func TestOptionalDateAndClasses(t *testing.T) {
	fields := map[string]string{}
	raw := "2026-11-11"
	got := optionalDate(fields, "active_till_date", &raw, nil)
	if got == nil || got.Format("2006-01-02") != "2026-11-11" || len(fields) != 0 {
		t.Fatalf("date %#v fields %#v", got, fields)
	}
	blank := " "
	if optionalDate(fields, "active_till_date", &blank, got) != nil {
		t.Fatal("blank date should clear")
	}
	bad := "tomorrow"
	if optionalDate(fields, "active_till_date", &bad, nil) != nil || fields["active_till_date"] == "" {
		t.Fatalf("bad date fields %#v", fields)
	}
	items := []string{" إنقاذ ", "إنقاذ", ""}
	classes := optionalClasses(map[string]string{}, &items, nil)
	if string(classes) != `["إنقاذ"]` {
		t.Fatalf("classes %s", classes)
	}
}

func intPtr(n int) *int { return &n }
