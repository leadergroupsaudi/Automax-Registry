package models

import (
	"encoding/json"
	"testing"
)

func TestDeviceIDAcceptsStringAndNumber(t *testing.T) {
	tests := []struct {
		body string
		want DeviceID
	}{
		{`{"deviceId":"dev-1"}`, "dev-1"},
		{`{"deviceId":42}`, "42"},
		{`{"deviceId":42.0}`, "42"},
		{`{"deviceId":null}`, ""},
	}
	for _, tt := range tests {
		var req PositionIngestRequest
		if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
			t.Fatalf("%s: %v", tt.body, err)
		}
		if req.DeviceID != tt.want {
			t.Fatalf("%s: deviceId = %q, want %q", tt.body, req.DeviceID, tt.want)
		}
	}
}
