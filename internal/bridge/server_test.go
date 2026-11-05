package bridge

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"kareya-order-bridge/internal/brendi"
	"kareya-order-bridge/internal/consumer"
)

// fakeBrendi simulates the Brendi API well enough to exercise the bridge.
type fakeBrendi struct {
	mu sync.Mutex

	events        []brendi.Event
	acked         []brendi.AckEvent
	confirms      []map[string]any
	cancellations []map[string]any
	delivered     int
	ready         int
	dispatched    int
}

func (f *fakeBrendi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case path == "/oauth/token":
		writeTestJSON(w, map[string]any{"access_token": "brendi-token", "expires_in": 3600})

	case path == "/v1/events:polling":
		if len(f.events) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeTestJSON(w, f.events)

	case path == "/v1/events/acknowledgment":
		var acks []brendi.AckEvent
		decodeTestJSON(r, &acks)
		f.acked = append(f.acked, acks...)
		acked := map[string]bool{}
		for _, ack := range acks {
			acked[ack.ID] = true
		}
		remaining := f.events[:0]
		for _, event := range f.events {
			if !acked[event.Key()] {
				remaining = append(remaining, event)
			}
		}
		f.events = remaining
		w.WriteHeader(http.StatusAccepted)

	case strings.HasSuffix(path, "/confirm"):
		var payload map[string]any
		decodeTestJSON(r, &payload)
		f.confirms = append(f.confirms, payload)
		w.WriteHeader(http.StatusOK)

	case strings.HasSuffix(path, "/requestCancellation"):
		var payload map[string]any
		decodeTestJSON(r, &payload)
		f.cancellations = append(f.cancellations, payload)
		w.WriteHeader(http.StatusOK)

	case strings.HasSuffix(path, "/delivered"):
		f.delivered++
		w.WriteHeader(http.StatusOK)

	case strings.HasSuffix(path, "/readyForPickup"):
		f.ready++
		w.WriteHeader(http.StatusOK)

	case strings.HasSuffix(path, "/dispatch"):
		f.dispatched++
		w.WriteHeader(http.StatusOK)

	case strings.HasPrefix(path, "/v1/orders/"):
		raw, err := os.ReadFile("../brendi/testdata/order.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)

	default:
		http.NotFound(w, r)
	}
}

func newTestBridge(t *testing.T) (*httptest.Server, *fakeBrendi) {
	t.Helper()

	fake := &fakeBrendi{
		events: []brendi.Event{
			{ID: "evt-1", EventType: brendi.EventCreated, OrderID: "order-1", CreatedAt: brendi.DateTime{}},
			{ID: "evt-2", EventType: brendi.EventDelivered, OrderID: "order-1", CreatedAt: brendi.DateTime{}},
		},
	}
	brendiServer := httptest.NewServer(fake)
	t.Cleanup(brendiServer.Close)

	client := brendi.New(brendiServer.URL, "client-id", "client-secret")
	server := New(Config{
		Token:        "secret",
		StoreID:      "store-1",
		StoreName:    "Kareya",
		Products:     consumer.ProductMap{ByName: map[string]string{"Donburi katsudon": "112"}},
		AckOnDetails: true,
	}, client, slog.New(slog.NewTextHandler(io.Discard, nil)))

	bridgeServer := httptest.NewServer(server.Handler())
	t.Cleanup(bridgeServer.Close)
	return bridgeServer, fake
}

func TestAuth(t *testing.T) {
	server, _ := newTestBridge(t)

	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		{"no token", func(*http.Request) {}, http.StatusUnauthorized},
		{"wrong token", func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }, http.StatusUnauthorized},
		{"bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer secret") }, http.StatusOK},
		{"api key", func(r *http.Request) { r.Header.Set("X-Api-Key", "secret") }, http.StatusOK},
		{"consumer xapikey", func(r *http.Request) { r.Header.Set("Xapikey", "secret") }, http.StatusOK},
		{"lowercase xapikey", func(r *http.Request) { r.Header["xapikey"] = []string{"secret"} }, http.StatusOK},
		{"query", func(*http.Request) {}, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := server.URL + "/v1/stores"
			if tc.name == "query" {
				url += "?token=secret"
			}
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(req)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestPollingOnlyForwardsCreated(t *testing.T) {
	server, fake := newTestBridge(t)

	resp := doRequest(t, http.MethodGet, server.URL+"/v1/events:polling", "secret")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var body consumer.PollingResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %d, want 1: %+v", len(body.Items), body.Items)
	}
	if body.Items[0].ID != "evt-1" || body.Items[0].OrderID != "order-1" {
		t.Errorf("event = %+v", body.Items[0])
	}
	if body.Items[0].Code != "PLC" || body.Items[0].FullCode != "PLACED" {
		t.Errorf("code = %q/%q", body.Items[0].Code, body.Items[0].FullCode)
	}
	if body.StatusCode != 0 {
		t.Errorf("statusCode = %d", body.StatusCode)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.acked) != 1 || fake.acked[0].ID != "evt-2" {
		t.Errorf("acks = %+v, want only evt-2 (DELIVERED)", fake.acked)
	}
	if len(fake.events) != 1 || fake.events[0].Key() != "evt-1" {
		t.Errorf("remaining events = %+v", fake.events)
	}
}

func TestOrderDetailsTranslatesAndAcks(t *testing.T) {
	server, fake := newTestBridge(t)

	resp := doRequest(t, http.MethodGet, server.URL+"/v1/orders/order-1", "secret")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}

	var body consumer.OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Item.DisplayID != "661009" {
		t.Errorf("displayId = %q", body.Item.DisplayID)
	}
	if body.Item.Total.OrderAmount != 38.89 {
		t.Errorf("orderAmount = %v", body.Item.Total.OrderAmount)
	}
	if len(body.Item.Items) != 1 || body.Item.Items[0].ExternalCode != "112" {
		t.Errorf("items = %+v", body.Item.Items)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.events) != 0 {
		t.Errorf("pending events = %+v, want none after details", fake.events)
	}
}

func TestStatusConfirm(t *testing.T) {
	server, fake := newTestBridge(t)

	body := `{"status":"CONFIRMED","justification":"accepted at the counter"}`
	resp := doRequest(t, http.MethodPost, server.URL+"/v1/orders/order-1/status", "secret", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.confirms) != 1 {
		t.Fatalf("confirms = %+v", fake.confirms)
	}
	confirm := fake.confirms[0]
	if confirm["reason"] != "accepted at the counter" {
		t.Errorf("reason = %v", confirm["reason"])
	}
	if confirm["orderExternalCode"] != "661009" {
		t.Errorf("orderExternalCode = %v, want displayId", confirm["orderExternalCode"])
	}
	if confirm["createdAt"] == "" {
		t.Error("empty createdAt")
	}
	if len(fake.events) != 0 {
		t.Errorf("pending events = %+v", fake.events)
	}
}

func TestStatusCancel(t *testing.T) {
	server, fake := newTestBridge(t)

	body := `{"orderId":"order-1","status":"CANCELLED","justification":"no courier"}`
	resp := doRequest(t, http.MethodPost, server.URL+"/v1/status", "secret", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.cancellations) != 1 {
		t.Fatalf("cancellations = %+v", fake.cancellations)
	}
	cancel := fake.cancellations[0]
	if cancel["reason"] != "no courier" || cancel["mode"] != "MANUAL" {
		t.Errorf("cancellation = %+v", cancel)
	}
}

func TestStatusConcludedMapsToDelivered(t *testing.T) {
	server, fake := newTestBridge(t)

	body := `{"status":"CONCLUDED"}`
	resp := doRequest(t, http.MethodPost, server.URL+"/v1/orders/order-1/status", "secret", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.delivered != 1 {
		t.Errorf("delivered = %d, want 1", fake.delivered)
	}
}

func TestStatusViaPathParameters(t *testing.T) {
	server, fake := newTestBridge(t)

	resp := doRequest(t, http.MethodPost, server.URL+"/v1/orders/order-1/status/CONFIRMED?justification=ok", "secret")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.confirms) != 1 {
		t.Fatalf("confirms = %+v", fake.confirms)
	}
	if fake.confirms[0]["reason"] != "ok" {
		t.Errorf("reason = %v", fake.confirms[0]["reason"])
	}
}

func TestUnsupportedStatus(t *testing.T) {
	server, _ := newTestBridge(t)

	body := `{"status":"WHATEVER"}`
	resp := doRequest(t, http.MethodPost, server.URL+"/v1/orders/order-1/status", "secret", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestStores(t *testing.T) {
	server, _ := newTestBridge(t)

	resp := doRequest(t, http.MethodGet, server.URL+"/v1/stores", "secret")
	defer resp.Body.Close()

	var body consumer.StoresResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].ID != "store-1" || body.Items[0].Name != "Kareya" {
		t.Errorf("stores = %+v", body.Items)
	}
}

func TestHealthzNeedsNoToken(t *testing.T) {
	server, _ := newTestBridge(t)

	resp := doRequest(t, http.MethodGet, server.URL+"/healthz", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func doRequest(t *testing.T, method, url, token string, body ...string) *http.Response {
	t.Helper()

	var reader io.Reader
	if len(body) > 0 {
		reader = strings.NewReader(body[0])
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func writeTestJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeTestJSON(r *http.Request, target any) {
	_ = json.NewDecoder(r.Body).Decode(target)
}
