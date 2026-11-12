package services

import (
	"testing"
	"time"
)

func clock(days string, start, end string) clockWindow {
	_, parsed, msg := normalizeDays(days)
	if msg != "" {
		panic(msg)
	}
	a, ok := parseClock(start)
	if !ok {
		panic(start)
	}
	b, ok := parseClock(end)
	if !ok {
		panic(end)
	}
	return clockWindow{days: parsed, start: a, end: b}
}

func at(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

func ptrTime(value string) *time.Time {
	t := at(value)
	return &t
}

func TestNormalizeDays(t *testing.T) {
	got, _, msg := normalizeDays(" mon, tue ,MON ")
	if msg == "" || got != "" {
		t.Fatalf("duplicate: %q %q", got, msg)
	}
	got, days, msg := normalizeDays("fri,mon")
	if msg != "" || got != "FRI,MON" || len(days) != 2 {
		t.Fatalf("got %q %#v %q", got, days, msg)
	}
	if _, _, msg := normalizeDays("MON,TUE,WED,THU,FRI,SAT"); msg == "" {
		t.Fatal("expected the 20 character limit to reject six days")
	}
}

func TestShiftOverlap(t *testing.T) {
	morning := clock("MON,TUE", "09:00", "12:00")
	late := clock("MON", "11:00", "13:00")
	afternoon := clock("MON", "13:00", "17:00")
	if !shiftsOverlap(morning, late) {
		t.Fatal("expected morning and late morning to overlap")
	}
	if shiftsOverlap(morning, afternoon) {
		t.Fatal("touching shifts should not overlap")
	}
	night := clock("MON", "22:00", "06:00")
	early := clock("TUE", "05:00", "07:00")
	after := clock("TUE", "06:00", "08:00")
	if !shiftsOverlap(night, early) {
		t.Fatal("expected overnight shift to overlap Tuesday morning")
	}
	if shiftsOverlap(night, after) {
		t.Fatal("expected 06:00 boundary to stay free")
	}
	sunday := clock("SUN", "22:00", "06:00")
	monday := clock("MON", "05:00", "07:00")
	if !shiftsOverlap(sunday, monday) {
		t.Fatal("expected Sunday night to overlap Monday morning")
	}
}

func TestAbsoluteOverlap(t *testing.T) {
	weekly := clock("MON", "09:00", "17:00")
	inside := occupancy{start: at("2026-10-26T10:00:00Z"), end: ptrTime("2026-10-26T11:00:00Z")}
	outside := occupancy{start: at("2026-10-26T18:00:00Z"), end: ptrTime("2026-10-26T19:00:00Z")}
	if !occupanciesOverlap(occupancy{weekly: &weekly}, inside) {
		t.Fatal("expected the Monday shift to cover 10:00")
	}
	if occupanciesOverlap(occupancy{weekly: &weekly}, outside) {
		t.Fatal("18:00 is outside the Monday shift")
	}
	open := occupancy{start: at("2026-10-26T18:00:00Z")}
	if !occupanciesOverlap(occupancy{weekly: &weekly}, open) {
		t.Fatal("an open-ended assignment overlaps a weekly shift")
	}
	left := occupancy{start: at("2026-10-26T10:00:00Z"), end: ptrTime("2026-10-26T12:00:00Z")}
	right := occupancy{start: at("2026-10-26T12:00:00Z"), end: ptrTime("2026-10-26T14:00:00Z")}
	if occupanciesOverlap(left, right) {
		t.Fatal("back-to-back windows should not overlap")
	}
	mid := occupancy{start: at("2026-10-26T11:00:00Z"), end: ptrTime("2026-10-26T13:00:00Z")}
	if !occupanciesOverlap(left, mid) {
		t.Fatal("expected the windows to overlap")
	}
}
