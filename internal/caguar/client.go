package caguar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxErrorBody = 512

// Device is the body Caguar stores for POST /api/devices and PUT /api/devices/{id}.
type Device struct {
	ID            int            `json:"id,omitempty"`
	Name          string         `json:"name"`
	UniqueID      string         `json:"uniqueId"`
	Status        string         `json:"status,omitempty"`
	Disabled      bool           `json:"disabled"`
	Category      string         `json:"category"`
	TrackerType   string         `json:"trackerType"`
	PlateNumber   string         `json:"plateNumber"`
	ChassisNumber string         `json:"chassisnumber"`
	Phone         string         `json:"phone"`
	Model         string         `json:"model"`
	Contact       string         `json:"contact"`
	GroupIDs      []int          `json:"groupIds,omitempty"`
	ZoneIDs       []int          `json:"zoneIds"`
	CalendarID    int            `json:"calendarId"`
	Attributes    map[string]any `json:"attributes"`
	// Product tells Caguar this device was created by Automax, so the portal does not edit or delete it.
	Product string `json:"product,omitempty"`
}

// ProductAutomax is the product value sent on every device create and update.
const ProductAutomax = "automax"

// Driver is the body Caguar stores for POST /api/drivers.
type Driver struct {
	Name     string `json:"name"`
	UniqueID string `json:"uniqueId"`
}

// Position is the latest fix Caguar returns from GET /api/positions.
type Position struct {
	DeviceID  int     `json:"deviceId"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	FixTime   string  `json:"fixTime"`
	Valid     *bool   `json:"valid"`
}

// RemoteError is a non-success response from the Caguar portal.
type RemoteError struct {
	Status int
	Body   string
}

func (e *RemoteError) Error() string {
	if e == nil {
		return "caguar error"
	}
	return fmt.Sprintf("caguar status %d", e.Status)
}

// The portal sits behind an edge that rejects clients with no browser user agent.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"

// Client posts device and driver registration to the Caguar portal.
type Client struct {
	baseURL  string
	token    string
	username string
	password string
	http     *http.Client
	mu       sync.Mutex
}

func New(baseURL, token string) *Client {
	return NewAccount(baseURL, token, "", "")
}

// NewAccount uses a bearer token when one is set. Otherwise it signs in with
// the portal username and password and keeps the issued token.
func NewAccount(baseURL, token, username, password string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL:  baseURL,
		token:    strings.TrimSpace(token),
		username: strings.TrimSpace(username),
		password: password,
		http:     &http.Client{Timeout: 20 * time.Second, Jar: jar},
	}
}

func (c *Client) RegisterDevice(ctx context.Context, device Device) error {
	// An empty groupIds array makes Caguar return a generic 400. Omit it unless groups were chosen.
	if len(device.GroupIDs) == 0 {
		device.GroupIDs = nil
	}
	if device.ZoneIDs == nil {
		device.ZoneIDs = []int{}
	}
	if device.Attributes == nil {
		device.Attributes = map[string]any{}
	}
	device.Product = ProductAutomax
	return c.post(ctx, "/api/devices", device)
}

// UpdateDevice loads the portal device by its current unique id, then sends PUT /api/devices/{id}.
// Group, zone, calendar, and attribute values already stored in Caguar are kept.
// Status is replaced only when the caller set one.
func (c *Client) UpdateDevice(ctx context.Context, currentUniqueID string, device Device) error {
	currentUniqueID = strings.TrimSpace(currentUniqueID)
	if currentUniqueID == "" {
		currentUniqueID = device.UniqueID
	}
	found, err := c.findDevice(ctx, currentUniqueID)
	if err != nil && restrictedProduct(err) {
		found, err = c.findListedDevice(ctx, currentUniqueID)
	}
	if err != nil {
		return err
	}
	found.Name = device.Name
	found.UniqueID = device.UniqueID
	if strings.TrimSpace(device.Status) != "" {
		found.Status = device.Status
	}
	found.Disabled = device.Disabled
	found.Category = device.Category
	found.TrackerType = device.TrackerType
	found.PlateNumber = device.PlateNumber
	found.ChassisNumber = device.ChassisNumber
	found.Phone = device.Phone
	found.Model = device.Model
	found.Contact = device.Contact
	if found.GroupIDs == nil {
		found.GroupIDs = []int{}
	}
	if found.ZoneIDs == nil {
		found.ZoneIDs = []int{}
	}
	if found.Attributes == nil {
		found.Attributes = map[string]any{}
	}
	// Leave product off the update. Caguar rejects a change that sends product=automax,
	// and the value stored at create time stays on the device.
	found.Product = ""
	_, err = c.send(ctx, http.MethodPut, "/api/devices/"+strconv.Itoa(found.ID), found)
	return err
}

func (c *Client) findListedDevice(ctx context.Context, uniqueID string) (Device, error) {
	devices, err := c.Devices(ctx)
	if err != nil {
		return Device{}, err
	}
	found, ok, err := pickDevice(devices, uniqueID)
	if err != nil {
		return Device{}, err
	}
	if !ok {
		return Device{}, &RemoteError{Status: http.StatusNotFound, Body: "device not found"}
	}
	return found, nil
}

func restrictedProduct(err error) bool {
	var remote *RemoteError
	if !errors.As(err, &remote) {
		return false
	}
	body := strings.ToLower(remote.Body)
	return strings.Contains(body, "restricted_product") || strings.Contains(body, "automax device")
}

// DeleteDevice removes the portal device with this tracker id.
// A missing device is a 404 from findDevice.
func (c *Client) DeleteDevice(ctx context.Context, uniqueID string) error {
	found, err := c.findDevice(ctx, strings.TrimSpace(uniqueID))
	if err != nil {
		return err
	}
	_, err = c.send(ctx, http.MethodDelete, "/api/devices/"+strconv.Itoa(found.ID), nil)
	return err
}

func (c *Client) RegisterDriver(ctx context.Context, driver Driver) error {
	return c.post(ctx, "/api/drivers", driver)
}

// Devices returns every device the signed-in account can see.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	raw, err := c.send(ctx, http.MethodGet, "/api/devices", nil)
	if err != nil {
		return nil, err
	}
	return decodeDevices(raw)
}

// Positions returns the latest fix for each visible device.
func (c *Client) Positions(ctx context.Context) ([]Position, error) {
	raw, err := c.send(ctx, http.MethodGet, "/api/positions", nil)
	if err != nil {
		return nil, err
	}
	return decodePositions(raw)
}

func (c *Client) post(ctx context.Context, path string, body any) error {
	_, err := c.send(ctx, http.MethodPost, path, body)
	return err
}

func (c *Client) findDevice(ctx context.Context, uniqueID string) (Device, error) {
	q := url.Values{}
	q.Set("uniqueId", uniqueID)
	raw, err := c.send(ctx, http.MethodGet, "/api/devices?"+q.Encode(), nil)
	if err != nil {
		return Device{}, err
	}
	found, ok, err := decodeDevice(raw, uniqueID)
	if err != nil {
		return Device{}, err
	}
	if !ok {
		return Device{}, &RemoteError{Status: http.StatusNotFound, Body: "device not found"}
	}
	return found, nil
}

func (c *Client) send(ctx context.Context, method, path string, body any) ([]byte, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("caguar is not configured")
	}
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	limit := maxErrorBody
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		limit = 1 << 20
	}
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &RemoteError{Status: resp.StatusCode, Body: strings.TrimSpace(string(payload))}
	}
	return payload, nil
}

func decodeDevice(raw []byte, uniqueID string) (Device, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return Device{}, false, nil
	}
	if raw[0] == '[' {
		var devices []Device
		if err := json.Unmarshal(raw, &devices); err != nil {
			return Device{}, false, err
		}
		return pickDevice(devices, uniqueID)
	}
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return Device{}, false, err
	}
	if len(bytes.TrimSpace(wrapped.Data)) > 0 && string(wrapped.Data) != "null" {
		return decodeDevice(wrapped.Data, uniqueID)
	}
	var one Device
	if err := json.Unmarshal(raw, &one); err != nil {
		return Device{}, false, err
	}
	if one.ID == 0 {
		return Device{}, false, nil
	}
	return one, true, nil
}

func decodeDevices(raw []byte) ([]Device, error) {
	list, err := decodeList(raw)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	var devices []Device
	if err := json.Unmarshal(list, &devices); err != nil {
		return nil, err
	}
	return devices, nil
}

func decodePositions(raw []byte) ([]Position, error) {
	list, err := decodeList(raw)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	var positions []Position
	if err := json.Unmarshal(list, &positions); err != nil {
		return nil, err
	}
	return positions, nil
}

func decodeList(raw []byte) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] == '[' {
		return raw, nil
	}
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, err
	}
	data := bytes.TrimSpace(wrapped.Data)
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	if data[0] == '[' {
		return data, nil
	}
	return nil, nil
}

func pickDevice(devices []Device, uniqueID string) (Device, bool, error) {
	for _, device := range devices {
		if device.ID != 0 && device.UniqueID == uniqueID {
			return device, true, nil
		}
	}
	if len(devices) == 1 && devices[0].ID != 0 {
		return devices[0], true, nil
	}
	return Device{}, false, nil
}

func (c *Client) ensureToken(ctx context.Context) error {
	if c.token != "" || c.username == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" {
		return nil
	}
	session := url.Values{}
	session.Set("email", c.username)
	session.Set("password", c.password)
	if _, err := c.form(ctx, "/api/session", session); err != nil {
		return err
	}
	expiration := time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	tokenBody := url.Values{}
	tokenBody.Set("expiration", expiration)
	token, err := c.form(ctx, "/api/session/token", tokenBody)
	if err != nil {
		return err
	}
	token, err = sessionToken(token)
	if err != nil {
		return err
	}
	c.token = token
	return nil
}

// sessionToken accepts the plain token older portals return, and the JSON
// body this portal returns: {"success":true,"data":"<token>"}.
func sessionToken(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("caguar session token was empty")
	}
	if !strings.HasPrefix(raw, "{") {
		return raw, nil
	}
	var parsed struct {
		Data    json.RawMessage `json:"data"`
		Token   string          `json:"token"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", fmt.Errorf("caguar session token was empty")
	}
	if len(parsed.Data) > 0 && string(parsed.Data) != "null" {
		var text string
		if err := json.Unmarshal(parsed.Data, &text); err == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), nil
		}
	}
	if strings.TrimSpace(parsed.Token) != "" {
		return strings.TrimSpace(parsed.Token), nil
	}
	if strings.TrimSpace(parsed.Message) != "" {
		return "", fmt.Errorf("caguar session token was empty: %s", strings.TrimSpace(parsed.Message))
	}
	return "", fmt.Errorf("caguar session token was empty")
}

func (c *Client) form(ctx context.Context, path string, values url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &RemoteError{Status: resp.StatusCode, Body: strings.TrimSpace(string(payload))}
	}
	return string(payload), nil
}
