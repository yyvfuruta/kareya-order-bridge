package main

import (
	"os"
	"path/filepath"
	"testing"
)

// unsetEnv removes the variable during the test and restores it afterward.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, exists := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if exists {
			os.Setenv(key, old)
			return
		}
		os.Unsetenv(key)
	})
}

// .env.local edited on Windows uses CRLF; loadEnv must handle it.
func TestLoadEnvWindowsLineEndings(t *testing.T) {
	for _, key := range []string{"BRIDGE_TEST_A", "BRIDGE_TEST_B", "BRIDGE_TEST_C", "BRIDGE_TEST_D"} {
		unsetEnv(t, key)
	}

	path := filepath.Join(t.TempDir(), ".env.local")
	content := "\ufeffBRIDGE_TEST_A=1\r\nBRIDGE_TEST_B=\"two\"\r\n# comment\r\nBRIDGE_TEST_C=with space \r\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := loadEnv(path); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"BRIDGE_TEST_A": "1",
		"BRIDGE_TEST_B": "two",
		"BRIDGE_TEST_C": "with space",
		"BRIDGE_TEST_D": "",
	}
	for key, want := range cases {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestLoadEnvDoesNotOverrideEnvironment(t *testing.T) {
	unsetEnv(t, "BRIDGE_TEST_KEEP")
	t.Setenv("BRIDGE_TEST_KEEP", "from-environment")

	path := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(path, []byte("BRIDGE_TEST_KEEP=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("BRIDGE_TEST_KEEP"); got != "from-environment" {
		t.Errorf("environment variable was overwritten: %q", got)
	}
}

func TestResolvePath(t *testing.T) {
	base := t.TempDir()

	if got := resolvePath("", base); got != "" {
		t.Errorf("empty = %q", got)
	}
	absolute := filepath.Join(base, "products.json")
	if got := resolvePath(absolute, base); got != absolute {
		t.Errorf("absolute = %q, want %q", got, absolute)
	}
	if got, want := resolvePath("products.json", base), filepath.Join(base, "products.json"); got != want {
		t.Errorf("relative = %q, want %q", got, want)
	}
}

func TestLoadProductsStripsBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "products.json")
	content := "\ufeff{\"byName\":{\"Donburi\":\"112\"}}"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	products, err := loadProducts(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := products.Lookup("", "Donburi"); got != "112" {
		t.Errorf("lookup = %q, want 112", got)
	}
}
