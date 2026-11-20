package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client()}
}

func TestListPingsDecodesWrappedResponse(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/checks/u-1/pings/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"pings":[{"type":"success","date":"2026-01-01T00:00:00+00:00","n":3,"method":"POST","duration":1.5},{"type":"start","n":2}]}`))
	})
	pings, raw, err := c.ListPings("u-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pings) != 2 || pings[0].N != 3 || pings[0].Method != "POST" {
		t.Errorf("pings = %+v", pings)
	}
	arr, err := extractArray(raw, "pings")
	if err != nil || len(arr) != 2 {
		t.Errorf("extractArray(pings) = %d elems, err %v; want 2", len(arr), err)
	}
}

func TestGetPingBody(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/checks/u-1/pings/3/body":
			w.Write([]byte("42%"))
		default:
			http.Error(w, "not found", 404)
		}
	})
	body, err := c.GetPingBody("u-1", 3)
	if err != nil || string(body) != "42%" {
		t.Errorf("body = %q, err %v; want verbatim 42%%", body, err)
	}
	_, err = c.GetPingBody("u-1", 9)
	if apiErr, ok := err.(*APIError); !ok || apiErr.Status != 404 {
		t.Errorf("err = %v; want 404 APIError", err)
	}
}
