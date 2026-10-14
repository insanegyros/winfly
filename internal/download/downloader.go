package download

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"jandy.cz/winfly/internal/config"
)

type Logger func(format string, args ...any)

var applyCacheSecurity = secureCacheDirectory

type Manager struct {
	cacheDirectory string
	maxBytes       int64
	allowHTTP      bool
	client         *http.Client
}

func New(network config.NetworkConfig, cacheDirectory string) (*Manager, error) {
	cacheDirectory = os.ExpandEnv(strings.TrimSpace(cacheDirectory))
	if cacheDirectory == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return nil, fmt.Errorf("určení adresáře mezipaměti: %w", err)
		}
		cacheDirectory = filepath.Join(base, "WinFly", "cache")
	}
	cacheDirectory = filepath.Clean(cacheDirectory)
	if !filepath.IsAbs(cacheDirectory) {
		absolute, err := filepath.Abs(cacheDirectory)
		if err != nil {
			return nil, fmt.Errorf("převod adresáře mezipaměti na absolutní cestu: %w", err)
		}
		cacheDirectory = absolute
	}
	if err := os.MkdirAll(cacheDirectory, 0o755); err != nil {
		return nil, fmt.Errorf("vytvoření adresáře mezipaměti %q: %w", cacheDirectory, err)
	}
	if err := applyCacheSecurity(cacheDirectory); err != nil {
		return nil, fmt.Errorf("zabezpečení adresáře mezipaměti %q: %w", cacheDirectory, err)
	}

	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("neočekávaný typ přenosu HTTP")
	}
	transport = transport.Clone()
	transport.Proxy = http.ProxyFromEnvironment

	timeout := time.Duration(network.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	maxBytes := network.MaxDownloadBytes
	if maxBytes <= 0 {
		maxBytes = config.DefaultMaxDownloadBytes
	}

	m := &Manager{
		cacheDirectory: cacheDirectory,
		maxBytes:       maxBytes,
		allowHTTP:      network.AllowHTTP,
	}
	m.client = &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("příliš mnoho přesměrování")
			}
			if !m.allowHTTP && strings.EqualFold(req.URL.Scheme, "http") {
				return errors.New("přesměrování na HTTP je zakázáno")
			}
			return nil
		},
	}
	return m, nil
}

func (m *Manager) Fetch(ctx context.Context, app config.AppConfig, log Logger) (string, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	if !config.IsSafeInstallerFileName(app.FileName) {
		return "", errors.New("nebezpečný název souboru instalátoru")
	}
	if err := m.validateScheme(app.URL); err != nil {
		return "", err
	}

	verifyDigest := app.HasValidSHA256()
	var expected []byte
	if verifyDigest {
		expected, _ = hex.DecodeString(strings.TrimSpace(app.SHA256))
	} else {
		log("Upozornění: aplikace %s nemá platný SHA-256; ověřování otisku je vypnuto", app.Name)
	}

	target := filepath.Join(m.cacheDirectory, app.FileName)
	if verifyDigest {
		if ok, err := fileMatchesSHA256(target, expected); err == nil && ok {
			log("Používá se ověřený instalátor z mezipaměti pro aplikaci %s", app.Name)
			return target, nil
		}
	}

	log("Stahuje se aplikace %s", app.Name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.URL, nil)
	if err != nil {
		return "", fmt.Errorf("vytvoření požadavku na stažení: %w", err)
	}
	req.Header.Set("User-Agent", "WinFly/1.1")
	req.Header.Set("Accept", "application/octet-stream, application/x-msdownload, */*")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("stažení aplikace %s: %w", app.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("stažení aplikace %s: stav HTTP %d", app.Name, resp.StatusCode)
	}
	if resp.ContentLength > m.maxBytes {
		return "", fmt.Errorf("stažení aplikace %s: deklarovaná velikost %d překračuje limit %d", app.Name, resp.ContentLength, m.maxBytes)
	}

	tmp, err := os.CreateTemp(m.cacheDirectory, app.FileName+".*.part")
	if err != nil {
		return "", fmt.Errorf("vytvoření dočasného souboru pro stažení: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	h := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, m.maxBytes+1))
	if copyErr != nil {
		return "", fmt.Errorf("zápis stažených dat: %w", copyErr)
	}
	if written > m.maxBytes {
		return "", fmt.Errorf("stažení aplikace %s překročilo limit velikosti %d", app.Name, m.maxBytes)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("zápis stažených dat na disk: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("uzavření staženého souboru: %w", err)
	}

	actual := h.Sum(nil)
	if verifyDigest && subtle.ConstantTimeCompare(actual, expected) != 1 {
		return "", fmt.Errorf("stažení aplikace %s neprošlo ověřením SHA-256; skutečný otisk je %x", app.Name, actual)
	}

	_ = os.Remove(target)
	if err := os.Rename(tmpName, target); err != nil {
		return "", fmt.Errorf("uložení instalátoru: %w", err)
	}
	committed = true
	if verifyDigest {
		log("Aplikace %s byla ověřena (%d bajtů)", app.Name, written)
	} else {
		log("Aplikace %s byla stažena bez ověření SHA-256 (%d bajtů, SHA-256 %x)", app.Name, written, actual)
	}
	return target, nil
}

func (m *Manager) validateScheme(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("URL instalátoru má neplatný zápis")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if m.allowHTTP {
			return nil
		}
		return errors.New("URL instalátoru používající HTTP jsou zakázány")
	default:
		return fmt.Errorf("nepodporované schéma URL instalátoru %q", u.Scheme)
	}
}

func fileMatchesSHA256(path string, expected []byte) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(h.Sum(nil), expected) == 1, nil
}
