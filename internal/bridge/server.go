// Package bridge expoe a API do parceiro que o Consumer consome, traduzindo
// os pedidos da Brendi e repassando as mudancas de status.
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

// Config e a configuracao do servidor.
type Config struct {
	// Token que o Consumer envia em toda requisicao.
	Token string
	// StoreID/StoreName aparecem no endpoint de listagem de lojas.
	StoreID   string
	StoreName string
	// Products traduz item da Brendi para o codigo PDV do Consumer.
	Products consumer.ProductMap
	// AckOnDetails confirma o evento na Brendi depois de entregar os detalhes.
	AckOnDetails bool
	// PollAllEvents reenvia qualquer evento como PLACED, nao so CREATED.
	PollAllEvents bool
	// LogHeaders loga os headers das requisicoes nao autenticadas (debug).
	LogHeaders bool
}

// Server implementa os endpoints do parceiro.
type Server struct {
	cfg    Config
	brendi *brendi.Client
	log    *slog.Logger
}

// New cria o servidor.
func New(cfg Config, client *brendi.Client, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, brendi: client, log: log}
}

// Handler devolve o http.Handler com as rotas.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.Handle("GET /v1/events:polling", s.auth(http.HandlerFunc(s.handlePolling)))
	// Algumas versoes do Consumer chamam esse campo de "URL de eventos" e
	// apontam para /v1/events; tratamos como o mesmo polling.
	mux.Handle("GET /v1/events", s.auth(http.HandlerFunc(s.handlePolling)))
	mux.Handle("GET /v1/events/{eventID}", s.auth(http.HandlerFunc(s.handleEvent)))
	mux.Handle("GET /v1/orders/{orderID}", s.auth(http.HandlerFunc(s.handleOrder)))
	mux.Handle("POST /v1/orders/{orderID}/status", s.auth(http.HandlerFunc(s.handleStatus)))
	// O manual fala em orderId/status como path parameters, mas o exemplo
	// manda tudo no corpo. Aceitamos as duas formas.
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
		// O Consumer so entende pedido novo. Os demais eventos sao
		// confirmados para nao voltarem eternamente no polling.
		if event.EventType != brendi.EventCreated && !s.cfg.PollAllEvents {
			ignored = append(ignored, event)
			continue
		}
		items = append(items, consumer.PollingEvent{
			ID:        event.Key(),
			OrderID:   event.OrderID,
			CreatedAt: event.CreatedAt.String(),
			FullCode:  "PLACED",
			Code:      "PLC",
		})
	}

	if len(ignored) > 0 {
		if err := s.brendi.Ack(r.Context(), ignored); err != nil {
			s.log.Warn("falha ao confirmar eventos ignorados", "err", err)
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
		writeJSON(w, http.StatusOK, consumer.EventResponse{
			Item: consumer.PollingEvent{
				ID:        event.Key(),
				OrderID:   event.OrderID,
				CreatedAt: event.CreatedAt.String(),
				FullCode:  "PLACED",
				Code:      "PLC",
			},
			StatusCode: 0,
		})
		return
	}
	s.failMsg(w, http.StatusNotFound, "evento nao encontrado")
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
		s.fail(w, http.StatusBadGateway, fmt.Errorf("pedido invalido: %w", err))
		return
	}

	writeJSON(w, http.StatusOK, consumer.OrderResponse{
		Item:       consumer.FromBrendi(&order, s.cfg.Products),
		StatusCode: 0,
	})

	// Ack depois de entregar os detalhes: se o Consumer nunca chegar aqui,
	// o evento continua na fila da Brendi.
	if s.cfg.AckOnDetails {
		s.ackOrder(orderID)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	var req consumer.StatusRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		s.failMsg(w, http.StatusBadRequest, "corpo invalido: "+err.Error())
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
		s.failMsg(w, http.StatusBadRequest, "orderId obrigatorio")
		return
	}

	// O status pode vir no corpo, no path ou na query.
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
	case "READY_FOR_PICKUP":
		err = s.brendi.ReadyForPickup(ctx, orderID)
	case "DISPATCHED":
		err = s.brendi.Dispatch(ctx, orderID)
	case "CONCLUDED", "DELIVERED", "FINISHED":
		err = s.brendi.Delivered(ctx, orderID)
	default:
		s.failMsg(w, http.StatusBadRequest, "status nao suportado: "+req.Status)
		return
	}

	if err != nil {
		s.fail(w, http.StatusBadGateway, err)
		return
	}

	s.ackOrder(orderID)

	msg := fmt.Sprintf("%s alterado para %q", orderID, status)
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

// ackOrder confirma na Brendi todos os eventos pendentes do pedido.
func (s *Server) ackOrder(orderID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	events, err := s.brendi.Poll(ctx)
	if err != nil {
		s.log.Warn("ack: polling falhou", "orderId", orderID, "err", err)
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
		s.log.Warn("ack: acknowledgment falhou", "orderId", orderID, "err", err)
		return
	}
	s.log.Info("eventos confirmados na Brendi", "orderId", orderID, "count", len(pending))
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
		s.log.Warn("token invalido", attrs...)
		s.failMsg(w, http.StatusUnauthorized, "token invalido")
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
	for _, header := range []string{"X-Api-Key", "Api-Key", "X-Token", "Token"} {
		if value := r.Header.Get(header); value != "" {
			candidates = append(candidates, value)
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
	s.log.Error("erro no proxy", "status", status, "err", err)
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
