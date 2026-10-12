package models

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// DeviceID is the Caugar device id. Their payload may send it as a string or a number.
type DeviceID string

func (d *DeviceID) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*d = ""
		return nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		*d = DeviceID(strings.TrimSpace(s))
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var n json.Number
	if err := dec.Decode(&n); err != nil {
		return err
	}
	*d = DeviceID(formatDeviceNumber(n))
	return nil
}

func formatDeviceNumber(n json.Number) string {
	if i, err := n.Int64(); err == nil {
		return strconv.FormatInt(i, 10)
	}
	f, err := n.Float64()
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return n.String()
	}
	if f == math.Trunc(f) && f <= math.MaxInt64 && f >= math.MinInt64 {
		return strconv.FormatInt(int64(f), 10)
	}
	return n.String()
}

// PositionIngestRequest is the live-position body Caugar POSTs to the adapter.
type PositionIngestRequest struct {
	DeviceID   DeviceID `json:"deviceId"`
	DeviceName string   `json:"deviceName"`
	Latitude   *float64 `json:"latitude"`
	Longitude  *float64 `json:"longitude"`
	FixTime    string   `json:"fixTime"`
	Valid      *bool    `json:"valid"`
}

// ExternalRefRequest links a registry thing to a Caugar device id.
type ExternalRefRequest struct {
	ExternalSystem string `json:"external_system"`
	ExternalID     string `json:"external_id"`
}
