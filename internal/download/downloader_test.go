package download

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"jandy.cz/ess-provisioner/internal/config"
)

func disableCacheSecurityForTest(t *testing.T) {
	t.Helper()
	original := applyCacheSecurity
	applyCacheSecurity = func(string) error { return nil }
	t.Cleanup(func() { applyCacheSecurity = original })
}

func TestFetchVerifiesAndCaches(t *testing.T) {
	disableCacheSecurityForTest(t)
	payload := []byte("testovaci obsah podepsaneho instalatoru")
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	manager, err := New(config.NetworkConfig{
		TimeoutSeconds:   10,
		MaxDownloadBytes: 1024,
		AllowHTTP:        true,
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := config.AppConfig{
		Name:     "Testovací aplikace",
		URL:      server.URL + "/installer.exe",
		SHA256:   digest,
		FileName: "installer.exe",
	}
	path, err := manager.Fetch(context.Background(), app, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("obsah = %q, očekáváno %q", got, payload)
	}
	if _, err := manager.Fetch(context.Background(), app, nil); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("počet požadavků = %d, očekáváno 1 díky ověřené mezipaměti", requests)
	}
}

func TestFetchRejectsHashMismatch(t *testing.T) {
	disableCacheSecurityForTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("obsah"))
	}))
	defer server.Close()

	manager, err := New(config.NetworkConfig{AllowHTTP: true, MaxDownloadBytes: 1024}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := config.AppConfig{
		Name:     "Testovací aplikace",
		URL:      server.URL,
		SHA256:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FileName: "installer.exe",
	}
	if _, err := manager.Fetch(context.Background(), app, nil); err == nil {
		t.Fatal("Fetch() neočekávaně přijal nesouhlasící otisk")
	}
}

func TestFetchAllowsMissingOrInvalidHashWithoutCacheReuse(t *testing.T) {
	for _, sha := range []string{"", "neplatny"} {
		t.Run(sha, func(t *testing.T) {
			disableCacheSecurityForTest(t)
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				_, _ = w.Write([]byte("obsah"))
			}))
			defer server.Close()

			manager, err := New(config.NetworkConfig{AllowHTTP: true, MaxDownloadBytes: 1024}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			app := config.AppConfig{Name: "Testovací aplikace", URL: server.URL, SHA256: sha, FileName: "installer.exe"}
			if _, err := manager.Fetch(context.Background(), app, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Fetch(context.Background(), app, nil); err != nil {
				t.Fatal(err)
			}
			if requests != 2 {
				t.Fatalf("počet požadavků = %d, očekáváno 2, protože neověřená mezipaměť se znovu nepoužívá", requests)
			}
		})
	}
}
