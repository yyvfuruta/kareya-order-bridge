// Package bridge exposes the partner API consumed by Consumer, translating
// Brendi orders and forwarding status changes.
package bridge

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"kareya-order-bridge/internal/brendi"
	"kareya-order-bridge/internal/consumer"
)

// Config is the server configuration.
type Config struct {
	// Token sent by Consumer on every request.
	Token string
	// Version is shown by /healthz (injected at build time).
	Version string
	// StoreID/StoreName are returned by the store listing endpoint.
	StoreID   string
	StoreName string
	// Products maps a Brendi item to the Consumer PDV code.
	Products consumer.ProductMap
	// AckOnDetails acknowledges the Brendi event after delivering the details.
	AckOnDetails bool
	// PollAllEvents forwards every event as PLACED, not only CREATED.
	PollAllEvents bool
	// LogHeaders logs headers of unauthenticated requests (debug).
	LogHeaders bool
}

// Server implements the partner endpoints.
type Server struct {
	cfg    Config
	brendi *brendi.Client
	log    *slog.Logger
}

// New creates the server.
func New(cfg Config, client *brendi.Client, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, brendi: client, log: log}
}

// Handler returns the http.Handler with the routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		payload := map[string]string{"status": "ok"}
		if s.cfg.Version != "" {
			payload["version"] = s.cfg.Version
		}
		writeJSON(w, http.StatusOK, payload)
	})

	mux.Handle("GET /v1/events:polling", s.auth(http.HandlerFunc(s.handlePolling)))
	// Some Consumer versions call this field "events URL" and point it to
	// /v1/events; treat it as the same polling endpoint.
	mux.Handle("GET /v1/events", s.auth(http.HandlerFunc(s.handlePolling)))
	mux.Handle("GET /v1/events/{eventID}", s.auth(http.HandlerFunc(s.handleEvent)))
	mux.Handle("GET /v1/orders/{orderID}", s.auth(http.HandlerFunc(s.handleOrder)))
	mux.Handle("POST /v1/orders/{orderID}/status", s.auth(http.HandlerFunc(s.handleStatus)))
	// The manual describes orderId/status as path parameters, but the example
	// sends everything in the body. Accept both forms.
	mux.Handle("POST /v1/orders/{orderID}/status/{status}", s.auth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("POST /v1/status", s.auth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("POST /v1/status/{orderID}", s.auth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("POST /v1/status/{orderID}/{status}", s.auth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("GET /v1/stores", s.auth(http.HandlerFunc(s.handleStores)))

	return s.logRequests(mux)
}

func (s *Server) handlePolling(w http.ResponseWriter, r *http.Request) {
	events, err := s.brendi.Poll(r.Context())
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	items := make([]consumer.PollingEvent, 0, len(events))
	var ignored []brendi.Event

	for _, event := range events {
		if event.OrderID == "" {
			ignored = append(ignored, event)
			continue
		}
		// Consumer only understands new orders. Other events are acknowledged
		// so they do not keep coming back on every poll.
		if event.EventType != brendi.EventCreated && !s.cfg.PollAllEvents {
			ignored = append(ignored, event)
			continue
		}
		createdAt := event.CreatedAt.ISO8601()
		if createdAt == "" {
			createdAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		}
		items = append(items, consumer.PollingEvent{
			ID:         event.Key(),
			OrderID:    event.OrderID,
			CreatedAt:  createdAt,
			FullCode:   "PLACED",
			Code:       "PLC",
			StatusCode: 0,
		})
	}

	if len(ignored) > 0 {
		if err := s.brendi.Ack(r.Context(), ignored); err != nil {
			s.log.Warn("failed to acknowledge ignored events", "err", err)
		}
	}

	writeJSON(w, http.StatusOK, consumer.PollingResponse{Items: items, StatusCode: 0})
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	events, err := s.brendi.Poll(r.Context())
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}
	for _, event := range events {
		if event.Key() != eventID {
			continue
		}
		createdAt := event.CreatedAt.ISO8601()
		if createdAt == "" {
			createdAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		}
		writeJSON(w, http.StatusOK, consumer.EventResponse{
			Item: consumer.PollingEvent{
				ID:         event.Key(),
				OrderID:    event.OrderID,
				CreatedAt:  createdAt,
				FullCode:   "PLACED",
				Code:       "PLC",
				StatusCode: 0,
			},
			StatusCode: 0,
		})
		return
	}
	s.failMsg(w, http.StatusNotFound, "event not found")
}

func (s *Server) handleOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderID")

	raw, err := s.brendi.OrderRaw(r.Context(), orderID)
	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	var order brendi.Order
	if err := json.Unmarshal(raw, &order); err != nil {
		s.fail(w, http.StatusBadGateway, fmt.Errorf("invalid order: %w", err))
		return
	}

	writeJSON(w, http.StatusOK, consumer.OrderResponse{
		Item:       consumer.FromBrendi(&order, s.cfg.Products),
		StatusCode: 0,
	})

	// Ack after delivering the details: if Consumer never gets here, the
	// event stays in the Brendi queue.
	if s.cfg.AckOnDetails {
		s.ackOrder(orderID)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var req consumer.StatusRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		s.failMsg(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	orderID := r.PathValue("orderID")
	if orderID == "" {
		orderID = req.OrderID
	}
	if orderID == "" {
		orderID = r.URL.Query().Get("orderId")
	}
	if orderID == "" {
		s.failMsg(w, http.StatusBadRequest, "orderId is required")
		return
	}

	// The status may come in the body, path, or query string.
	if req.Status == "" {
		req.Status = r.PathValue("status")
	}
	if req.Status == "" {
		req.Status = r.URL.Query().Get("status")
	}
	if req.Justification == "" {
		req.Justification = r.URL.Query().Get("justification")
	}

	ctx := r.Context()
	externalCode := orderID
	if order, err := s.brendi.Order(ctx, orderID); err == nil && order.DisplayID != "" {
		externalCode = order.DisplayID
	}

	status := strings.ToUpper(strings.TrimSpace(req.Status))
	var err error
	switch status {
	case "CONFIRMED":
		err = s.brendi.Confirm(ctx, orderID, req.Justification, externalCode, 0)
	case "CANCELLED", "CANCELED":
		err = s.brendi.RequestCancellation(ctx, orderID, req.Justification)
	case "READY_FOR_PICKUP", "READYTOPICKUP", "READY_TO_PICKUP":
		err = s.brendi.ReadyForPickup(ctx, orderID)
	case "DISPATCHED", "DISPATCH":
		err = s.brendi.Dispatch(ctx, orderID)
	case "CONCLUDED", "DELIVERED", "FINISHED", "FINALIZED":
		err = s.brendi.Delivered(ctx, orderID)
	default:
		s.failMsg(w, http.StatusBadRequest, "unsupported status: "+req.Status)
		return
	}

	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	s.ackOrder(orderID)

	msg := fmt.Sprintf("%s changed to %q", orderID, status)
	if req.Justification != "" {
		msg += ": " + req.Justification
	}
	writeJSON(w, http.StatusOK, consumer.Response{StatusCode: 0, ReasonPhrase: &msg})
}

func (s *Server) handleStores(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, consumer.StoresResponse{
		Items:      []consumer.Store{{ID: s.cfg.StoreID, Name: s.cfg.StoreName}},
		StatusCode: 0,
	})
}

// ackOrder acknowledges all pending events of the order in Brendi.
func (s *Server) ackOrder(orderID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	events, err := s.brendi.Poll(ctx)
	if err != nil {
		s.log.Warn("ack: polling failed", "orderId", orderID, "err", err)
		return
	}

	pending := make([]brendi.Event, 0, len(events))
	for _, event := range events {
		if event.OrderID == orderID {
			pending = append(pending, event)
		}
	}
	if len(pending) == 0 {
		return
	}
	if err := s.brendi.Ack(ctx, pending); err != nil {
		s.log.Warn("ack: acknowledgment failed", "orderId", orderID, "err", err)
		return
	}
	s.log.Info("events acknowledged in Brendi", "orderId", orderID, "count", len(pending))
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.tokenOK(r) {
			next.ServeHTTP(w, r)
			return
		}
		attrs := []any{"path", r.URL.Path, "remote", r.RemoteAddr}
		if s.cfg.LogHeaders {
			attrs = append(attrs, "headers", r.Header)
		}
		s.log.Warn("invalid token", attrs...)
		s.failMsg(w, http.StatusUnauthorized, "invalid token")
	})
}

func (s *Server) tokenOK(r *http.Request) bool {
	if s.cfg.Token == "" {
		return false
	}

	var candidates []string

	if auth := r.Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			candidates = append(candidates, strings.TrimSpace(parts[1]))
		} else {
			candidates = append(candidates, strings.TrimSpace(auth))
		}
	}
	for _, header := range []string{
		"X-Api-Key", "Api-Key", "X-Token", "Token",
		"Xapikey", "X-Apikey", "Apikey", "X-Access-Token",
	} {
		if value := r.Header.Get(header); value != "" {
			candidates = append(candidates, value)
		}
	}
	for k, values := range r.Header {
		norm := strings.ToLower(strings.ReplaceAll(k, "-", ""))
		if norm == "xapikey" || norm == "apikey" || norm == "xtoken" || norm == "token" || norm == "xaccesstoken" {
			candidates = append(candidates, values...)
		}
	}
	query := r.URL.Query()
	for _, key := range []string{"token", "access_token", "api_key"} {
		if value := query.Get(key); value != "" {
			candidates = append(candidates, value)
		}
	}

	for _, candidate := range candidates {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(s.cfg.Token)) == 1 {
			return true
		}
	}
	return false
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).Round(time.Millisecond).String(),
			"remote", r.RemoteAddr,
		)
	})
}

func (s *Server) fail(w http.ResponseWriter, status int, err error) {
	var apiErr *brendi.APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		status = http.StatusNotFound
	}
	s.log.Error("proxy error", "status", status, "err", err)
	s.failMsg(w, status, err.Error())
}

func (s *Server) failMsg(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, consumer.Response{StatusCode: status, ReasonPhrase: &msg})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
