// Command bridge exposes the partner API consumed by Consumer.
//
//	go run ./cmd/bridge
//
// Environment variables (or .env.local):
//
//	PORT                    HTTP port (default 8080)
//	BRIDGE_HOST             listen interface (default 127.0.0.1, local only)
//	BRIDGE_LOG_FILE         optional log file (append; no rotation)
//	BRIDGE_TOKEN            token required from Consumer (mandatory)
//	BRIDGE_STORE_ID         store id in the listing (default: BRENDI_CLIENT_ID)
//	BRIDGE_STORE_NAME       store name in the listing
//	BRIDGE_PRODUCT_MAP      path to the product mapping JSON (optional)
//	BRIDGE_ACK_ON_DETAILS   acknowledge the Brendi event when delivering details (default true)
//	BRIDGE_POLL_ALL_EVENTS  forward any event as PLACED (default false)
//	BRIDGE_LOG_HEADERS      log headers of unauthenticated requests (default false)
//	BRENDI_CLIENT_ID        Brendi credential (mandatory)
//	BRENDI_CLIENT_SECRET    Brendi credential (mandatory)
//	BRENDI_BASE_URL         Brendi API base URL (optional)
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"kareya-order-bridge/internal/brendi"
	"kareya-order-bridge/internal/bridge"
	"kareya-order-bridge/internal/consumer"
)

// version is injected by GoReleaser via -X main.version=...
var version = "dev"

func main() {
	envFile, baseDir := findEnvFile()
	if err := loadEnv(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", envFile, err)
		os.Exit(2)
	}

	logger, logFile := setupLogger()
	if logFile != nil {
		defer logFile.Close()
	}
	slog.SetDefault(logger)

	clientID := os.Getenv("BRENDI_CLIENT_ID")
	clientSecret := os.Getenv("BRENDI_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		fatal("set BRENDI_CLIENT_ID and BRENDI_CLIENT_SECRET")
	}

	token := os.Getenv("BRIDGE_TOKEN")
	if token == "" {
		fatal("set BRIDGE_TOKEN (token that Consumer will send)")
	}

	products, err := loadProducts(resolvePath(os.Getenv("BRIDGE_PRODUCT_MAP"), baseDir))
	if err != nil {
		fatal("error reading BRIDGE_PRODUCT_MAP: " + err.Error())
	}

	storeID := os.Getenv("BRIDGE_STORE_ID")
	if storeID == "" {
		storeID = clientID
	}

	cfg := bridge.Config{
		Token:         token,
		Version:       version,
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

	// Listen on localhost by default: Consumer runs on the same machine.
	// To expose it on the network, set BRIDGE_HOST=0.0.0.0 (and mind the firewall).
	host := os.Getenv("BRIDGE_HOST")
	if host == "" {
		host = "127.0.0.1"
	}

	httpServer := &http.Server{
		Addr:              net.JoinHostPort(host, port),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("bridge listening", "version", version, "addr", httpServer.Addr, "storeId", storeID)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("HTTP server: " + err.Error())
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
}

// findEnvFile looks for .env.local next to the executable (important when
// running as a Windows service, which may start in another directory) and
// falls back to the current directory.
func findEnvFile() (string, string) {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, ".env.local")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, dir
		}
	}
	return ".env.local", "."
}

// resolvePath resolves relative paths from the .env.local directory.
func resolvePath(path, baseDir string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// setupLogger writes to stdout and, if BRIDGE_LOG_FILE is set, also to the
// file (append, no rotation).
func setupLogger() (*slog.Logger, *os.File) {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	path := strings.TrimSpace(os.Getenv("BRIDGE_LOG_FILE"))
	if path == "" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts)), nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fatal("error opening BRIDGE_LOG_FILE: " + err.Error())
	}
	return slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, file), opts)), file
}

func loadProducts(path string) (consumer.ProductMap, error) {
	if path == "" {
		return consumer.ProductMap{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return consumer.ProductMap{}, err
	}
	// Files saved on Windows may start with a BOM.
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
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
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			// Notepad may save as UTF-8 with BOM.
			line = strings.TrimPrefix(line, "\ufeff")
			firstLine = false
		}
		line = strings.TrimSpace(line)
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
	slog.Error(msg)
	os.Exit(1)
}
