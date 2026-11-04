// Comando brendi-poll testa o fluxo de eventos/pedidos da Brendi:
//
//	token -> events:polling -> orders/:id -> (opcional) events/acknowledgment
//
// Uso:
//
//	go run ./cmd/brendi-poll           # baixa e salva, NAO confirma os eventos
//	go run ./cmd/brendi-poll -ack      # confirma os eventos depois de salvar tudo
//
// Credenciais vem de .env.local ou das variaveis BRENDI_CLIENT_ID,
// BRENDI_CLIENT_SECRET e BRENDI_BASE_URL.
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
		ackFlag  = flag.Bool("ack", false, "confirma (ack) os eventos depois de processar")
		outDir   = flag.String("out", "var/brendi", "diretorio para salvar os JSONs")
		showJSON = flag.Bool("json", false, "imprime o JSON completo dos pedidos")
	)
	flag.Parse()

	if err := loadEnv(".env.local"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "erro lendo .env.local: %v\n", err)
		os.Exit(2)
	}

	clientID := os.Getenv("BRENDI_CLIENT_ID")
	clientSecret := os.Getenv("BRENDI_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fmt.Fprintln(os.Stderr, "defina BRENDI_CLIENT_ID e BRENDI_CLIENT_SECRET (.env.local ou ambiente)")
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
		fmt.Println("GET /v1/events:polling -> 204, nenhum evento pendente (token e polling OK)")
		return
	}
	fmt.Printf("GET /v1/events:polling -> %d evento(s)\n", len(events))

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail("mkdir", err)
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	eventsFile := filepath.Join(*outDir, "events-"+stamp+".json")
	if err := writeJSON(eventsFile, events); err != nil {
		fail("salvar eventos", err)
	}
	fmt.Printf("eventos salvos em %s\n", eventsFile)

	for i, event := range events {
		fmt.Printf("\n[%d/%d] %s orderId=%s eventId=%s\n", i+1, len(events), event.EventType, event.OrderID, event.Key())
		if event.OrderID == "" {
			fmt.Println("  evento sem orderId, pulando detalhes")
			continue
		}

		raw, err := client.OrderRaw(ctx, event.OrderID)
		if err != nil {
			fmt.Printf("  GET /v1/orders/%s falhou: %v\n", event.OrderID, err)
			continue
		}
		orderFile := filepath.Join(*outDir, "order-"+event.OrderID+".json")
		if err := os.WriteFile(orderFile, raw, 0o644); err != nil {
			fmt.Printf("  salvar pedido: %v\n", err)
			continue
		}
		fmt.Printf("  GET /v1/orders/%s -> OK, salvo em %s\n", event.OrderID, orderFile)

		var order brendi.Order
		if err := json.Unmarshal(raw, &order); err != nil {
			fmt.Printf("  aviso: pedido nao decodificou nas structs: %v\n", err)
		} else {
			printOrderSummary(&order)
		}
		if *showJSON {
			fmt.Println(indent(raw))
		}
	}

	if !*ackFlag {
		fmt.Println("\neventos NAO confirmados (rode com -ack quando o processamento estiver ok)")
		return
	}

	if err := client.Ack(ctx, events); err != nil {
		fail("acknowledgment", err)
	}
	fmt.Printf("\nPOST /v1/events/acknowledgment -> OK (%d evento(s) confirmado(s))\n", len(events))
}

func printOrderSummary(o *brendi.Order) {
	total := o.Total.OrderAmount
	fmt.Printf("  pedido %s | %s | status %s | total %.2f %s\n", o.DisplayID, o.Type, o.LastEvent, total.Value, total.Currency)
	if o.Customer != nil {
		fmt.Printf("  cliente: %s (%s)\n", o.Customer.Name, o.Customer.Phone.Number)
	}
	for _, item := range o.Items {
		fmt.Printf("  item: %gx %s = %.2f\n", item.Quantity, item.Name, item.TotalPrice.Value)
	}
	switch {
	case o.Delivery != nil:
		fmt.Printf("  entrega: %s (%.6f, %.6f)\n", o.Delivery.DeliveryAddress.FormattedAddress,
			o.Delivery.DeliveryAddress.Coordinates.Latitude, o.Delivery.DeliveryAddress.Coordinates.Longitude)
	case o.Takeout != nil:
		fmt.Printf("  retirada em %s\n", o.Takeout.TakeoutDateTime)
	case o.Indoor != nil:
		fmt.Printf("  salao: mesa %s\n", o.Indoor.Tab)
	}
	fmt.Printf("  pagamento: prepaid=%.2f pendente=%.2f\n", o.Payments.Prepaid, o.Payments.Pending)
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
	fmt.Fprintf(os.Stderr, "%s falhou: %v\n", step, err)
	os.Exit(1)
}
