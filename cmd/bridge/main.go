// Comando bridge expoe a API de parceiro que o Consumer consome.
//
//	go run ./cmd/bridge
//
// Variaveis de ambiente (ou .env.local):
//
//	PORT                    porta HTTP (padrao 8080)
//	BRIDGE_HOST             interface de escuta (padrao 127.0.0.1, so local)
//	BRIDGE_LOG_FILE         arquivo de log opcional (append; sem rotacao)
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

// version e injetada pelo GoReleaser via -X main.version=...
var version = "dev"

func main() {
	envFile, baseDir := findEnvFile()
	if err := loadEnv(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "erro lendo %s: %v\n", envFile, err)
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
		fatal("defina BRENDI_CLIENT_ID e BRENDI_CLIENT_SECRET")
	}

	token := os.Getenv("BRIDGE_TOKEN")
	if token == "" {
		fatal("defina BRIDGE_TOKEN (token que o Consumer vai enviar)")
	}

	products, err := loadProducts(resolvePath(os.Getenv("BRIDGE_PRODUCT_MAP"), baseDir))
	if err != nil {
		fatal("erro lendo BRIDGE_PRODUCT_MAP: " + err.Error())
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

	// Por padrao escuta so em localhost: o Consumer roda na mesma maquina.
	// Para expor na rede, defina BRIDGE_HOST=0.0.0.0 (e cuide do firewall).
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
		logger.Info("bridge no ar", "version", version, "addr", httpServer.Addr, "storeId", storeID)
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

// findEnvFile procura .env.local ao lado do executavel (importante quando roda
// como servico do Windows, que pode iniciar em outro diretorio) e cai para o
// diretorio atual.
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

// resolvePath resolve caminhos relativos a partir do diretorio do .env.local.
func resolvePath(path, baseDir string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// setupLogger escreve no stdout e, se BRIDGE_LOG_FILE estiver definido, tambem
// no arquivo (append, sem rotacao).
func setupLogger() (*slog.Logger, *os.File) {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}

	path := strings.TrimSpace(os.Getenv("BRIDGE_LOG_FILE"))
	if path == "" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts)), nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fatal("erro abrindo BRIDGE_LOG_FILE: " + err.Error())
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, file), opts)), file
}

func loadProducts(path string) (consumer.ProductMap, error) {
	if path == "" {
		return consumer.ProductMap{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return consumer.ProductMap{}, err
	}
	// Arquivo salvo no Windows pode vir com BOM.
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
			// Notepad pode salvar como UTF-8 com BOM.
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
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
