// Command brendi-poll tests the Brendi event/order flow:
//
//	token -> events:polling -> orders/:id -> (optional) events/acknowledgment
//
// Usage:
//
//	go run ./cmd/brendi-poll           # download and save, does NOT acknowledge events
//	go run ./cmd/brendi-poll -ack      # acknowledge events after saving everything
//
// Credentials come from .env.local or the BRENDI_CLIENT_ID,
// BRENDI_CLIENT_SECRET, and BRENDI_BASE_URL variables.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kareya-order-bridge/internal/brendi"
)

func main() {
	var (
		ackFlag  = flag.Bool("ack", false, "acknowledge events after processing")
		outDir   = flag.String("out", "var/brendi", "directory to save the JSON files")
		showJSON = flag.Bool("json", false, "print the full order JSON")
	)
	flag.Parse()

	if err := loadEnv(".env.local"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "error reading .env.local: %v\n", err)
		os.Exit(2)
	}

	clientID := os.Getenv("BRENDI_CLIENT_ID")
	clientSecret := os.Getenv("BRENDI_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fmt.Fprintln(os.Stderr, "set BRENDI_CLIENT_ID and BRENDI_CLIENT_SECRET (.env.local or environment)")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := brendi.New(os.Getenv("BRENDI_BASE_URL"), clientID, clientSecret)

	token, err := client.Token(ctx)
	if err != nil {
		fail("token", err)
	}
	fmt.Printf("token OK (%s...%s)\n", token[:6], token[len(token)-4:])

	events, err := client.Poll(ctx)
	if err != nil {
		fail("polling", err)
	}
	if len(events) == 0 {
		fmt.Println("GET /v1/events:polling -> 204, no pending events (token and polling OK)")
		return
	}
	fmt.Printf("GET /v1/events:polling -> %d event(s)\n", len(events))

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail("mkdir", err)
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	eventsFile := filepath.Join(*outDir, "events-"+stamp+".json")
	if err := writeJSON(eventsFile, events); err != nil {
		fail("save events", err)
	}
	fmt.Printf("events saved to %s\n", eventsFile)

	for i, event := range events {
		fmt.Printf("\n[%d/%d] %s orderId=%s eventId=%s\n", i+1, len(events), event.EventType, event.OrderID, event.Key())
		if event.OrderID == "" {
			fmt.Println("  event without orderId, skipping details")
			continue
		}

		raw, err := client.OrderRaw(ctx, event.OrderID)
		if err != nil {
			fmt.Printf("  GET /v1/orders/%s failed: %v\n", event.OrderID, err)
			continue
		}
		orderFile := filepath.Join(*outDir, "order-"+event.OrderID+".json")
		if err := os.WriteFile(orderFile, raw, 0o644); err != nil {
			fmt.Printf("  save order: %v\n", err)
			continue
		}
		fmt.Printf("  GET /v1/orders/%s -> OK, saved to %s\n", event.OrderID, orderFile)

		var order brendi.Order
		if err := json.Unmarshal(raw, &order); err != nil {
			fmt.Printf("  warning: order did not decode into the structs: %v\n", err)
		} else {
			printOrderSummary(&order)
		}
		if *showJSON {
			fmt.Println(indent(raw))
		}
	}

	if !*ackFlag {
		fmt.Println("\nevents NOT acknowledged (run with -ack once processing is OK)")
		return
	}

	if err := client.Ack(ctx, events); err != nil {
		fail("acknowledgment", err)
	}
	fmt.Printf("\nPOST /v1/events/acknowledgment -> OK (%d event(s) acknowledged)\n", len(events))
}

func printOrderSummary(o *brendi.Order) {
	total := o.Total.OrderAmount
	fmt.Printf("  order %s | %s | status %s | total %.2f %s\n", o.DisplayID, o.Type, o.LastEvent, total.Value, total.Currency)
	if o.Customer != nil {
		fmt.Printf("  customer: %s (%s)\n", o.Customer.Name, o.Customer.Phone.Number)
	}
	for _, item := range o.Items {
		fmt.Printf("  item: %gx %s = %.2f\n", item.Quantity, item.Name, item.TotalPrice.Value)
	}
	switch {
	case o.Delivery != nil:
		fmt.Printf("  delivery: %s (%.6f, %.6f)\n", o.Delivery.DeliveryAddress.FormattedAddress,
			o.Delivery.DeliveryAddress.Coordinates.Latitude, o.Delivery.DeliveryAddress.Coordinates.Longitude)
	case o.Takeout != nil:
		fmt.Printf("  pickup at %s\n", o.Takeout.TakeoutDateTime)
	case o.Indoor != nil:
		fmt.Printf("  dine-in: table %s\n", o.Indoor.Tab)
	}
	fmt.Printf("  payment: prepaid=%.2f pending=%.2f\n", o.Payments.Prepaid, o.Payments.Pending)
}

func loadEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func indent(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "%s failed: %v\n", step, err)
	os.Exit(1)
}
