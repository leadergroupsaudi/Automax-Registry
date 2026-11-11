package caguar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxErrorBody = 512

// Device is the body Caguar stores for POST /api/devices.
type Device struct {
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
	GroupIDs      []int          `json:"groupIds"`
	ZoneIDs       []int          `json:"zoneIds"`
	CalendarID    int            `json:"calendarId"`
	Attributes    map[string]any `json:"attributes"`
}

// Driver is the body Caguar stores for POST /api/drivers.
type Driver struct {
	Name     string `json:"name"`
	UniqueID string `json:"uniqueId"`
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
	if device.GroupIDs == nil {
		device.GroupIDs = []int{}
	}
	if device.ZoneIDs == nil {
		device.ZoneIDs = []int{}
	}
	if device.Attributes == nil {
		device.Attributes = map[string]any{}
	}
	return c.post(ctx, "/api/devices", device)
}

func (c *Client) RegisterDriver(ctx context.Context, driver Driver) error {
	return c.post(ctx, "/api/drivers", driver)
}

func (c *Client) post(ctx context.Context, path string, body any) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("caguar is not configured")
	}
	if err := c.ensureToken(ctx); err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &RemoteError{Status: resp.StatusCode, Body: strings.TrimSpace(string(payload))}
	}
	return nil
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
