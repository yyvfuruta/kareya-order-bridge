// Package brendi is a minimal client for the Brendi integration API.
//
// Supported flow:
//
//	token (client_credentials) -> events:polling -> orders/:id -> events/acknowledgment
package brendi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultBaseURL = "https://api.brendi.com.br"

// APIError represents a Brendi HTTP error response.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("brendi: HTTP %d: %s", e.Status, strings.TrimSpace(e.Body))
}

// Client talks to the Brendi API and caches the access token.
type Client struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// New creates a client. An empty baseURL uses https://api.brendi.com.br.
func New(baseURL, clientID, clientSecret string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Token returns a valid access token, refreshing it when less than 60s remain.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}

	form := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", &APIError{Status: resp.StatusCode, Body: string(body)}
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("brendi: invalid token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", errors.New("brendi: token response without access_token")
	}

	c.token = tr.AccessToken
	lifetime := time.Duration(tr.ExpiresIn) * time.Second
	if lifetime <= 60*time.Second {
		lifetime = 60 * time.Second
	}
	c.tokenExp = time.Now().Add(lifetime - 60*time.Second)
	return c.token, nil
}

// Poll fetches pending events. Returns an empty slice when the API responds 204.
func (c *Client) Poll(ctx context.Context) ([]Event, error) {
	status, body, err := c.do(ctx, http.MethodGet, "/v1/events:polling", nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent {
		return []Event{}, nil
	}
	if status != http.StatusOK {
		return nil, &APIError{Status: status, Body: string(body)}
	}

	var events []Event
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("brendi: invalid events: %w", err)
	}
	return events, nil
}

// OrderRaw fetches the order details and returns the raw JSON.
func (c *Client) OrderRaw(ctx context.Context, orderID string) (json.RawMessage, error) {
	status, body, err := c.do(ctx, http.MethodGet, "/v1/orders/"+url.PathEscape(orderID), nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &APIError{Status: status, Body: string(body)}
	}
	return json.RawMessage(body), nil
}

// Order fetches and decodes the order details.
func (c *Client) Order(ctx context.Context, orderID string) (*Order, error) {
	raw, err := c.OrderRaw(ctx, orderID)
	if err != nil {
		return nil, err
	}
	var order Order
	if err := json.Unmarshal(raw, &order); err != nil {
		return nil, fmt.Errorf("brendi: invalid order: %w", err)
	}
	return &order, nil
}

// Ack acknowledges that the events were processed. They no longer show up in polling afterward.
func (c *Client) Ack(ctx context.Context, events []Event) error {
	acks := make([]AckEvent, len(events))
	for i, event := range events {
		acks[i] = event.Ack()
	}
	payload, err := json.Marshal(acks)
	if err != nil {
		return err
	}
	status, body, err := c.do(ctx, http.MethodPost, "/v1/events/acknowledgment", payload)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent:
		return nil
	default:
		return &APIError{Status: status, Body: string(body)}
	}
}

// Confirm confirms order receipt in Brendi.
func (c *Client) Confirm(ctx context.Context, orderID, reason, orderExternalCode string, preparationMinutes int) error {
	payload := map[string]any{
		"createdAt":         time.Now().UTC().Format(time.RFC3339),
		"orderExternalCode": orderExternalCode,
	}
	if reason != "" {
		payload["reason"] = reason
	}
	if preparationMinutes > 0 {
		payload["preparationTime"] = preparationMinutes
	}
	return c.postExpect(ctx, "/v1/orders/"+url.PathEscape(orderID)+"/confirm", payload)
}

// ReadyForPickup tells Brendi the order is ready for pickup.
func (c *Client) ReadyForPickup(ctx context.Context, orderID string) error {
	return c.postExpect(ctx, "/v1/orders/"+url.PathEscape(orderID)+"/readyForPickup", map[string]any{})
}

// Dispatch tells Brendi the order is out for delivery.
func (c *Client) Dispatch(ctx context.Context, orderID string) error {
	return c.postExpect(ctx, "/v1/orders/"+url.PathEscape(orderID)+"/dispatch", map[string]any{})
}

// Delivered tells Brendi the order was delivered/completed.
func (c *Client) Delivered(ctx context.Context, orderID string) error {
	return c.postExpect(ctx, "/v1/orders/"+url.PathEscape(orderID)+"/delivered", map[string]any{})
}

// RequestCancellation requests order cancellation.
func (c *Client) RequestCancellation(ctx context.Context, orderID, reason string) error {
	if reason == "" {
		reason = "Cancelled by the restaurant in Consumer"
	}
	payload := map[string]any{
		"reason": reason,
		"code":   "INTERNAL_DIFFICULTIES_OF_THE_RESTAURANT",
		"mode":   "MANUAL",
	}
	return c.postExpect(ctx, "/v1/orders/"+url.PathEscape(orderID)+"/requestCancellation", payload)
}

func (c *Client) postExpect(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	status, respBody, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return nil
	// 409/422 usually mean "already in this state"; there is no point in
	// Consumer retrying the call.
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return nil
	default:
		return &APIError{Status: status, Body: string(respBody)}
	}
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	token, err := c.Token(ctx)
	if err != nil {
		return 0, nil, err
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}
