// Comando bridge expoe a API de parceiro que o Consumer consome.
//
//	go run ./cmd/bridge
//
// Variaveis de ambiente (ou .env.local):
//
//	PORT                    porta HTTP (padrao 8080)
//	BRIDGE_TOKEN            token exigido do Consumer (obrigatorio)
//	BRIDGE_STORE_ID         id da loja na listagem (padrao: BRENDI_CLIENT_ID)
//	BRIDGE_STORE_NAME       nome da loja na listagem
//	BRIDGE_PRODUCT_MAP      caminho do JSON de de-para de produtos (opcional)
//	BRIDGE_ACK_ON_DETAILS   confirma o evento na Brendi ao entregar detalhes (padrao true)
//	BRIDGE_POLL_ALL_EVENTS  reenvia qualquer evento como PLACED (padrao false)
//	BRIDGE_LOG_HEADERS      loga headers de requisicoes sem token (padrao false)
//	BRENDI_CLIENT_ID        credencial Brendi (obrigatoria)
//	BRENDI_CLIENT_SECRET    credencial Brendi (obrigatoria)
//	BRENDI_BASE_URL         base da API Brendi (opcional)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"kareya-order-bridge/internal/brendi"
	"kareya-order-bridge/internal/bridge"
	"kareya-order-bridge/internal/consumer"
)

func main() {
	if err := loadEnv(".env.local"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "erro lendo .env.local: %v\n", err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	clientID := os.Getenv("BRENDI_CLIENT_ID")
	clientSecret := os.Getenv("BRENDI_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fatal("defina BRENDI_CLIENT_ID e BRENDI_CLIENT_SECRET")
	}

	token := os.Getenv("BRIDGE_TOKEN")
	if token == "" {
		fatal("defina BRIDGE_TOKEN (token que o Consumer vai enviar)")
	}

	products, err := loadProducts(os.Getenv("BRIDGE_PRODUCT_MAP"))
	if err != nil {
		fatal("erro lendo BRIDGE_PRODUCT_MAP: " + err.Error())
	}

	storeID := os.Getenv("BRIDGE_STORE_ID")
	if storeID == "" {
		storeID = clientID
	}

	cfg := bridge.Config{
		Token:         token,
		StoreID:       storeID,
		StoreName:     os.Getenv("BRIDGE_STORE_NAME"),
		Products:      products,
		AckOnDetails:  envBool("BRIDGE_ACK_ON_DETAILS", true),
		PollAllEvents: envBool("BRIDGE_POLL_ALL_EVENTS", false),
		LogHeaders:    envBool("BRIDGE_LOG_HEADERS", false),
	}

	client := brendi.New(os.Getenv("BRENDI_BASE_URL"), clientID, clientSecret)
	server := bridge.New(cfg, client, logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("bridge no ar", "addr", httpServer.Addr, "storeId", storeID)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("servidor HTTP: " + err.Error())
		}
	}()

	<-ctx.Done()
	logger.Info("encerrando...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
}

func loadProducts(path string) (consumer.ProductMap, error) {
	if path == "" {
		return consumer.ProductMap{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return consumer.ProductMap{}, err
	}
	var products consumer.ProductMap
	if err := json.Unmarshal(data, &products); err != nil {
		return consumer.ProductMap{}, err
	}
	if products.ByExternalCode == nil {
		products.ByExternalCode = map[string]string{}
	}
	if products.ByName == nil {
		products.ByName = map[string]string{}
	}
	return products, nil
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
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

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
