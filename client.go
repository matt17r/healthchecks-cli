package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a thin wrapper around the healthchecks.io Management API v3.
type Client struct {
	BaseURL string // e.g. https://healthchecks.io (no trailing /api/v3)
	APIKey  string
	HTTP    *http.Client
}

// APIError is returned for non-2xx responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Body)
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("API error %d: %s", e.Status, msg)
}

func newClient(cfg *Config) *Client {
	return &Client{
		BaseURL: strings.TrimRight(cfg.BaseURL, "/"),
		APIKey:  cfg.APIKey,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// do performs a request and returns the raw response body. path is relative to
// the /api/v3/ root, e.g. "checks/" or "checks/<uuid>/pings/".
func (c *Client) do(method, path string, body any) ([]byte, error) {
	url := c.BaseURL + "/api/v3/" + strings.TrimLeft(path, "/")

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(buf)
	}

	for i := 0; i < len(c.APIKey); i++ {
		if b := c.APIKey[i]; b < 0x20 || b == 0x7f {
			return nil, fmt.Errorf("API key contains invalid characters (control bytes) — re-enter it, avoiding pasted whitespace or escape sequences")
		}
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(data)}
	}
	return data, nil
}

// ---- Types ----

// Check models a check from the API. Fields absent in read-only responses
// (uuid, ping_url, ...) are simply left empty.
type Check struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Tags      string `json:"tags"`
	Desc      string `json:"desc"`
	Grace     int    `json:"grace"`
	NPings    int    `json:"n_pings"`
	Status    string `json:"status"`
	Started   bool   `json:"started"`
	LastPing  string `json:"last_ping"`
	NextPing  string `json:"next_ping"`
	Timeout   int    `json:"timeout"`
	Schedule  string `json:"schedule"`
	TZ        string `json:"tz"`
	UUID      string `json:"uuid"`
	UniqueKey string `json:"unique_key"`
	PingURL   string `json:"ping_url"`
	UpdateURL string `json:"update_url"`
	PauseURL  string `json:"pause_url"`
	ResumeURL string `json:"resume_url"`
	Channels  string `json:"channels"`
}

// ID returns the identifier usable in API paths: the uuid for read-write keys,
// or the unique_key for read-only keys.
func (c Check) ID() string {
	if c.UUID != "" {
		return c.UUID
	}
	return c.UniqueKey
}

type checksResponse struct {
	Checks []Check `json:"checks"`
}

type Ping struct {
	Type     string  `json:"type"`
	Date     string  `json:"date"`
	N        int     `json:"n"`
	Scheme   string  `json:"scheme"`
	Method   string  `json:"method"`
	RemoteIP string  `json:"remote_addr"`
	Duration float64 `json:"duration"`
}

type Flip struct {
	Timestamp string `json:"timestamp"`
	Up        int    `json:"up"`
}

type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type channelsResponse struct {
	Channels []Channel `json:"channels"`
}

// ---- API methods ----

func (c *Client) ListChecks(query string) ([]Check, []byte, error) {
	path := "checks/"
	if query != "" {
		path += "?" + query
	}
	data, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var out checksResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return out.Checks, data, nil
}

func (c *Client) GetCheck(id string) (*Check, []byte, error) {
	data, err := c.do(http.MethodGet, "checks/"+id, nil)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Status == 404 {
			return c.getCheckBySlug(id)
		}
		return nil, nil, err
	}
	var out Check
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return &out, data, nil
}

func (c *Client) getCheckBySlug(slug string) (*Check, []byte, error) {
	checks, _, err := c.ListChecks("slug=" + url.QueryEscape(slug))
	if err != nil {
		return nil, nil, err
	}
	if len(checks) == 0 {
		return nil, nil, &APIError{Status: 404, Body: "no check found with uuid, unique_key, or slug " + slug}
	}
	if len(checks) > 1 {
		return nil, nil, fmt.Errorf("%d checks match slug %q — use the uuid to disambiguate", len(checks), slug)
	}
	ck := checks[0]
	raw, err := json.Marshal(ck)
	if err != nil {
		return nil, nil, err
	}
	return &ck, raw, nil
}

func (c *Client) ListPings(id string) ([]Ping, []byte, error) {
	data, err := c.do(http.MethodGet, "checks/"+id+"/pings/", nil)
	if err != nil {
		return nil, nil, err
	}
	var out []Ping
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return out, data, nil
}

func (c *Client) ListFlips(id string) ([]Flip, []byte, error) {
	data, err := c.do(http.MethodGet, "checks/"+id+"/flips/", nil)
	if err != nil {
		return nil, nil, err
	}
	var out []Flip
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return out, data, nil
}

func (c *Client) ListChannels() ([]Channel, []byte, error) {
	data, err := c.do(http.MethodGet, "channels/", nil)
	if err != nil {
		return nil, nil, err
	}
	var out channelsResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return out.Channels, data, nil
}

func (c *Client) Status() ([]byte, error) {
	return c.do(http.MethodGet, "status/", nil)
}

// Write operations.

func (c *Client) CreateCheck(body map[string]any) (*Check, []byte, error) {
	data, err := c.do(http.MethodPost, "checks/", body)
	if err != nil {
		return nil, nil, err
	}
	var out Check
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return &out, data, nil
}

func (c *Client) UpdateCheck(id string, body map[string]any) (*Check, []byte, error) {
	data, err := c.do(http.MethodPost, "checks/"+id, body)
	if err != nil {
		return nil, nil, err
	}
	var out Check
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return &out, data, nil
}

func (c *Client) PauseCheck(id string) (*Check, []byte, error) {
	data, err := c.do(http.MethodPost, "checks/"+id+"/pause", nil)
	if err != nil {
		return nil, nil, err
	}
	var out Check
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return &out, data, nil
}

func (c *Client) ResumeCheck(id string) (*Check, []byte, error) {
	data, err := c.do(http.MethodPost, "checks/"+id+"/resume", nil)
	if err != nil {
		return nil, nil, err
	}
	var out Check
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return &out, data, nil
}

func (c *Client) DeleteCheck(id string) (*Check, []byte, error) {
	data, err := c.do(http.MethodDelete, "checks/"+id, nil)
	if err != nil {
		return nil, nil, err
	}
	var out Check
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, data, err
	}
	return &out, data, nil
}
