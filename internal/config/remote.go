package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const userAgent = "WinFly/1.1"

// FetchRemoteApps stáhne a ověří aktuální seznam aplikací. Nekontroluje
// příponu URL ani Content-Type, takže koncovým bodem může být například PHP.
func FetchRemoteApps(ctx context.Context, cfg *Config) ([]AppConfig, error) {
	if cfg == nil {
		return nil, errors.New("konfigurace je nil")
	}
	if err := validateURL(cfg.RemoteConfigURL, cfg.Network.AllowHTTP); err != nil {
		return nil, fmt.Errorf("neplatná adresa vzdálené konfigurace: %w", err)
	}

	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("neočekávaný typ výchozího přenosu HTTP")
	}
	transport = transport.Clone()
	transport.Proxy = http.ProxyFromEnvironment

	timeout := time.Duration(cfg.Network.ConfigTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxBytes := cfg.Network.MaxConfigBytes
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("příliš mnoho přesměrování")
			}
			if !cfg.Network.AllowHTTP && strings.EqualFold(req.URL.Scheme, "http") {
				return errors.New("přesměrování na HTTP je zakázáno")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.RemoteConfigURL, nil)
	if err != nil {
		return nil, fmt.Errorf("vytvoření požadavku na vzdálenou konfiguraci: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/json, text/plain, */*")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stažení vzdálené konfigurace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("stažení vzdálené konfigurace: stav HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf("vzdálená konfigurace má deklarovanou velikost %d bajtů, limit je %d", resp.ContentLength, maxBytes)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("čtení vzdálené konfigurace: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("vzdálená konfigurace překročila limit %d bajtů", maxBytes)
	}

	apps, err := LoadRemoteApps(data, cfg.Network)
	if err != nil {
		return nil, fmt.Errorf("vzdálená konfigurace není platná: %w", err)
	}
	return apps, nil
}
