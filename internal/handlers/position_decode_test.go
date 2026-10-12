package handlers

import "testing"

func TestDecodePositionIgnoresUnknownFields(t *testing.T) {
	body := []byte(`{"deviceId":"4289765883","deviceName":"Unit 1","latitude":12.9716,"longitude":77.5946,"fixTime":"2026-10-09T10:00:00Z","device":"erf"}`)
	req, ignored, err := decodePosition(body)
	if err != nil {
		t.Fatal(err)
	}
	if req.DeviceID != "4289765883" || req.DeviceName != "Unit 1" {
		t.Fatalf("req %#v", req)
	}
	if len(ignored) != 1 || ignored[0] != "device" {
		t.Fatalf("ignored %#v", ignored)
	}
}

func TestDecodePositionReadsTraccarDeviceName(t *testing.T) {
	body := []byte(`{"position":{"deviceId":42,"latitude":1,"longitude":2,"fixTime":"2026-10-09T10:00:00Z"},"device":{"name":"Unit 1","status":"online"}}`)
	req, ignored, err := decodePosition(body)
	if err != nil {
		t.Fatal(err)
	}
	if req.DeviceID != "42" || req.DeviceName != "Unit 1" {
		t.Fatalf("req %#v", req)
	}
	if len(ignored) != 0 {
		t.Fatalf("ignored %#v", ignored)
	}
}
