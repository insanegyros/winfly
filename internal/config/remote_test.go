package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchRemoteAppsAcceptsPHPStyleEndpointAndAnyContentType(t *testing.T) {
	var receivedPath string
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		if got := r.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("Cache-Control = %q", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`{"apps":[{"id":1,"name":"Aplikace","enabled":true,"url":"https://jandy.cz/app.exe","fileName":"app.exe"}]}`))
	}))
	defer server.Close()

	cfg := &Config{
		RemoteConfigURL: server.URL + "/apps.php?get=applist",
		Network: NetworkConfig{
			AllowHTTP:            true,
			ConfigTimeoutSeconds: 5,
			MaxConfigBytes:       4096,
		},
	}
	apps, err := FetchRemoteApps(context.Background(), cfg)
	if err != nil {
		t.Fatalf("FetchRemoteApps() vrátil chybu: %v", err)
	}
	if len(apps) != 1 || apps[0].ID != 1 || apps[0].Name != "Aplikace" {
		t.Fatalf("neočekávané aplikace: %#v", apps)
	}
	if receivedPath != "/apps.php" || receivedQuery != "get=applist" {
		t.Fatalf("požadavek dorazil na %q?%q", receivedPath, receivedQuery)
	}
}

func TestFetchRemoteAppsRejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?php toto není JSON`))
	}))
	defer server.Close()

	cfg := &Config{
		RemoteConfigURL: server.URL + "/apps.php",
		Network:         NetworkConfig{AllowHTTP: true, MaxConfigBytes: 4096},
	}
	apps, err := FetchRemoteApps(context.Background(), cfg)
	if err == nil {
		t.Fatal("FetchRemoteApps() neočekávaně přijal neplatný JSON")
	}
	if apps != nil {
		t.Fatalf("při chybě má být výsledek nil, získáno: %#v", apps)
	}
}

func TestFetchRemoteAppsRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nenalezeno", http.StatusNotFound)
	}))
	defer server.Close()

	cfg := &Config{
		RemoteConfigURL: server.URL,
		Network:         NetworkConfig{AllowHTTP: true, MaxConfigBytes: 4096},
	}
	_, err := FetchRemoteApps(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("očekávána chyba HTTP 404, získáno: %v", err)
	}
}

func TestFetchRemoteAppsRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"apps":[]}` + strings.Repeat(" ", 100)))
	}))
	defer server.Close()

	cfg := &Config{
		RemoteConfigURL: server.URL,
		Network:         NetworkConfig{AllowHTTP: true, MaxConfigBytes: 16},
	}
	if _, err := FetchRemoteApps(context.Background(), cfg); err == nil {
		t.Fatal("FetchRemoteApps() neočekávaně přijal nadlimitní odpověď")
	}
}
