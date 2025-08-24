package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aejkatappaja/goflux/internal/analytics"
	"github.com/aejkatappaja/goflux/internal/events"
)

type testServer struct {
	*httptest.Server
	bus *events.EventBus
	svc *analytics.Service
}

func setupTest(t *testing.T) *testServer {
	t.Helper()

	bus := events.NewEventBus()
	store := analytics.NewMemoryStore()
	svc := analytics.NewService(bus, store, []string{"view", "click", "signup"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start service: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("GET /analytics/types", analyticsTotalsHandler(svc))
	mux.Handle("GET /analytics/types/{type}", analyticsTypeHandler(svc))
	mux.Handle("POST /events", publishEventHandler(bus))

	srv := httptest.NewServer(cors(mux))
	t.Cleanup(srv.Close)

	return &testServer{
		Server: srv,
		bus:    bus,
		svc:    svc,
	}
}

func (ts *testServer) postJSON(t *testing.T, path string, payload any) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post request: %v", err)
	}

	return resp
}

func (ts *testServer) getJSON(t *testing.T, path string, result any) {
	t.Helper()

	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("got status %d, body: %s", resp.StatusCode, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestHealth(t *testing.T) {
	ts := setupTest(t)

	var result map[string]string
	ts.getJSON(t, "/health", &result)

	if result["status"] != "ok" {
		t.Errorf("health status = %q, want ok", result["status"])
	}
}

func TestPublishEvent_Success(t *testing.T) {
	ts := setupTest(t)

	tests := []struct {
		name    string
		payload map[string]any
		want    int
	}{
		{
			name:    "system event",
			payload: map[string]any{"type": "view", "payload": map[string]any{"page": "/"}},
			want:    http.StatusAccepted,
		},
		{
			name:    "user event",
			payload: map[string]any{"type": "click", "userId": "u123", "payload": map[string]any{}},
			want:    http.StatusAccepted,
		},
		{
			name:    "normalized type",
			payload: map[string]any{"type": " VIEW ", "payload": nil},
			want:    http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := ts.postJSON(t, "/events", tt.payload)
			defer resp.Body.Close()

			if resp.StatusCode != tt.want {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("status = %d, want %d, body: %s", resp.StatusCode, tt.want, body)
			}
		})
	}
}

func TestPublishEvent_Validation(t *testing.T) {
	ts := setupTest(t)

	tests := []struct {
		name    string
		payload string
		want    int
	}{
		{
			name:    "empty type",
			payload: `{"type":"","payload":{}}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "invalid json",
			payload: `{broken}`,
			want:    http.StatusBadRequest,
		},
		{
			name:    "missing type",
			payload: `{"payload":{}}`,
			want:    http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Post(
				ts.URL+"/events",
				"application/json",
				bytes.NewReader([]byte(tt.payload)),
			)
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestAnalytics_EndToEnd(t *testing.T) {
	ts := setupTest(t)

	events := []map[string]any{
		{"type": "view", "payload": map[string]any{}},
		{"type": "view", "payload": map[string]any{}},
		{"type": "click", "userId": "u1", "payload": map[string]any{}},
		{"type": "signup", "userId": "u2", "payload": map[string]any{}},
	}

	for _, evt := range events {
		resp := ts.postJSON(t, "/events", evt)
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("failed to publish event: %v", evt)
		}
	}

	t.Run("totals", func(t *testing.T) {
		var totals map[string]uint64
		ts.getJSON(t, "/analytics/types", &totals)

		expected := map[string]uint64{
			"view":   2,
			"click":  1,
			"signup": 1,
		}

		for typ, want := range expected {
			if got := totals[typ]; got != want {
				t.Errorf("totals[%s] = %d, want %d", typ, got, want)
			}
		}
	})

	t.Run("specific type", func(t *testing.T) {
		var result map[string]any
		ts.getJSON(t, "/analytics/types/view", &result)

		if result["type"] != "view" {
			t.Errorf("type = %v, want view", result["type"])
		}
		if count := result["count"].(float64); count != 2 {
			t.Errorf("count = %v, want 2", count)
		}
	})

	t.Run("untracked type", func(t *testing.T) {
		var result map[string]any
		ts.getJSON(t, "/analytics/types/unknown", &result)

		if count := result["count"].(float64); count != 0 {
			t.Errorf("untracked type count = %v, want 0", count)
		}
	})
}

func TestCORS(t *testing.T) {
	ts := setupTest(t)

	t.Run("preflight OPTIONS", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/events", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Content-Type")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("OPTIONS status = %d, want 204", resp.StatusCode)
		}

		checks := map[string]string{
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		}

		for header, want := range checks {
			if got := resp.Header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		}
	})

	t.Run("actual request", func(t *testing.T) {
		resp := ts.postJSON(t, "/events", map[string]any{
			"type":    "view",
			"payload": map[string]any{},
		})
		defer resp.Body.Close()

		if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "*" {
			t.Errorf("Allow-Origin = %q on actual request", origin)
		}
	})
}

func BenchmarkPublishEvent(b *testing.B) {
	// Setup comme avant...
	bus := events.NewEventBus()
	store := analytics.NewMemoryStore()
	svc := analytics.NewService(bus, store, []string{"view"})
	_ = svc.Start(context.Background())

	mux := http.NewServeMux()
	mux.Handle("POST /events", publishEventHandler(bus))

	handler := cors(mux)

	// Benchmark SANS réseau (direct handler)
	payload, _ := json.Marshal(map[string]any{
		"type":    "view",
		"userId":  "bench",
		"payload": map[string]any{},
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/events", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusAccepted {
				b.Errorf("got status %d", w.Code)
			}
		}
	})
}

func TestConcurrentRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	ts := setupTest(t)

	const concurrent = 100
	done := make(chan bool, concurrent)

	for i := range concurrent {
		go func(id int) {
			defer func() { done <- true }()

			payload := map[string]any{
				"type":    "view",
				"userId":  fmt.Sprintf("user-%d", id),
				"payload": map[string]any{},
			}

			resp := ts.postJSON(t, "/events", payload)
			resp.Body.Close()
		}(i)
	}

	for range concurrent {
		<-done
	}

	var totals map[string]uint64
	ts.getJSON(t, "/analytics/types", &totals)

	if totals["view"] != concurrent {
		t.Errorf("after %d concurrent requests, got %d views", concurrent, totals["view"])
	}
}
