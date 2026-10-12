package config

import (
	"bytes"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

//go:embed config.json
var embeddedFS embed.FS

// Config obsahuje nastavení vložené do spustitelného souboru při sestavení.
// Seznam Apps se doplní až po úspěšném načtení vzdálené konfigurace.
type Config struct {
	ProductName     string        `json:"productName"`
	CacheDirectory  string        `json:"cacheDirectory"`
	RemoteConfigURL string        `json:"remoteConfigUrl"`
	Network         NetworkConfig `json:"network"`
	Apps            []AppConfig   `json:"-"`
}

// NetworkConfig určuje síťové limity pro vzdálenou konfiguraci i instalátory.
type NetworkConfig struct {
	ConfigTimeoutSeconds int64 `json:"configTimeoutSeconds"`
	MaxConfigBytes       int64 `json:"maxConfigBytes"`
	TimeoutSeconds       int64 `json:"timeoutSeconds"`
	MaxDownloadBytes     int64 `json:"maxDownloadBytes"`
	AllowHTTP            bool  `json:"allowHttp"`
}

// AppConfig popisuje jednu aplikaci ve vzdáleném seznamu.
type AppConfig struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Enabled          *bool    `json:"enabled"`
	URL              string   `json:"url"`
	SHA256           string   `json:"sha256"`
	FileName         string   `json:"fileName"`
	InstallerType    string   `json:"installerType"`
	Arguments        []string `json:"arguments"`
	Wait             *bool    `json:"wait"`
	SuccessExitCodes []int    `json:"successExitCodes"`
}

type remoteDocument struct {
	Apps []AppConfig `json:"apps"`
}

// LoadEmbedded načte a ověří konfigurační soubor vložený do aplikace.
func LoadEmbedded() (*Config, error) {
	data, err := embeddedFS.ReadFile("config.json")
	if err != nil {
		return nil, fmt.Errorf("načtení vložené konfigurace: %w", err)
	}
	return Load(data)
}

// Load načte a ověří vloženou konfiguraci ze zadaných dat.
func Load(data []byte) (*Config, error) {
	var cfg Config
	if err := decodeSingleJSON(data, &cfg, "vložené konfigurace"); err != nil {
		return nil, err
	}

	applyConfigDefaults(&cfg)
	return &cfg, nil
}

// LoadRemoteApps načte a ověří JSON vrácený vzdáleným koncovým bodem.
// URL ani Content-Type odpovědi nemají vliv; rozhodující je struktura těla.
func LoadRemoteApps(data []byte, network NetworkConfig) ([]AppConfig, error) {
	var document remoteDocument
	if err := decodeSingleJSON(data, &document, "vzdálené konfigurace"); err != nil {
		return nil, err
	}
	if document.Apps == nil {
		return nil, errors.New("vzdálená konfigurace neobsahuje povinné pole apps")
	}

	applyAppDefaults(document.Apps)
	if err := validateApps(document.Apps, network.AllowHTTP); err != nil {
		return nil, err
	}
	return document.Apps, nil
}

func decodeSingleJSON(data []byte, target any, description string) error {
	data = bytes.TrimSpace(data)
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	data = bytes.TrimSpace(data)

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("dekódování %s: %w", description, localizedJSONError(err))
	}
	if err := ensureEOF(dec); err != nil {
		return fmt.Errorf("kontrola %s: %w", description, err)
	}
	return nil
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("obsahuje více hodnot JSON")
		}
		return fmt.Errorf("dekódování dat za hlavní hodnotou JSON: %w", localizedJSONError(err))
	}
	return nil
}

func localizedJSONError(err error) error {
	if errors.Is(err, io.EOF) {
		return errors.New("dokument JSON je prázdný")
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Errorf("chybná syntaxe JSON na bajtu %d", syntaxErr.Offset)
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field != "" {
			return fmt.Errorf("pole %q má nesprávný datový typ na bajtu %d", typeErr.Field, typeErr.Offset)
		}
		return fmt.Errorf("hodnota má nesprávný datový typ na bajtu %d", typeErr.Offset)
	}
	const unknownFieldPrefix = "json: unknown field "
	if message := err.Error(); strings.HasPrefix(message, unknownFieldPrefix) {
		return fmt.Errorf("neznámé pole %s", strings.TrimPrefix(message, unknownFieldPrefix))
	}
	return errors.New("neplatný dokument JSON")
}

func applyConfigDefaults(cfg *Config) {
	cfg.ProductName = strings.TrimSpace(cfg.ProductName)
	if cfg.ProductName == "" {
		cfg.ProductName = "ESSENS Provisioner"
	}
	cfg.CacheDirectory = strings.TrimSpace(cfg.CacheDirectory)
	if cfg.CacheDirectory == "" {
		cfg.CacheDirectory = `${ProgramData}\ESSProvisioner\cache`
	}
	cfg.RemoteConfigURL = strings.TrimSpace(cfg.RemoteConfigURL)
	if cfg.Network.ConfigTimeoutSeconds <= 0 {
		cfg.Network.ConfigTimeoutSeconds = 30
	}
	if cfg.Network.MaxConfigBytes <= 0 {
		cfg.Network.MaxConfigBytes = 1024 * 1024
	}
	if cfg.Network.TimeoutSeconds <= 0 {
		cfg.Network.TimeoutSeconds = 900
	}
	if cfg.Network.MaxDownloadBytes <= 0 {
		cfg.Network.MaxDownloadBytes = 2 * 1024 * 1024 * 1024
	}
}

func applyAppDefaults(apps []AppConfig) {
	for i := range apps {
		app := &apps[i]
		app.Name = strings.TrimSpace(app.Name)
		app.Description = strings.TrimSpace(app.Description)
		app.URL = strings.TrimSpace(app.URL)
		app.SHA256 = strings.ToLower(strings.TrimSpace(app.SHA256))
		app.FileName = strings.TrimSpace(app.FileName)
		app.InstallerType = strings.ToLower(strings.TrimSpace(app.InstallerType))
		if app.InstallerType == "" {
			app.InstallerType = "auto"
		}
		if app.Wait == nil {
			value := true
			app.Wait = &value
		}
		if len(app.SuccessExitCodes) == 0 {
			app.SuccessExitCodes = []int{0, 1641, 3010}
		}
	}
}

func validateApps(apps []AppConfig, allowHTTP bool) error {
	seen := make(map[int64]struct{}, len(apps))
	for i, app := range apps {
		path := fmt.Sprintf("apps[%d]", i)
		if app.ID <= 0 {
			return fmt.Errorf("pole %s.id musí být kladné celé číslo", path)
		}
		if _, exists := seen[app.ID]; exists {
			return fmt.Errorf("hodnota %s.id %d je duplicitní", path, app.ID)
		}
		seen[app.ID] = struct{}{}

		if app.Enabled == nil {
			return fmt.Errorf("pole %s.enabled je povinné", path)
		}
		// Zakázané položky mohou záměrně sloužit jen jako neúplné šablony.
		if !*app.Enabled {
			continue
		}
		if app.Name == "" {
			return fmt.Errorf("pole %s.name je u povolené aplikace povinné", path)
		}
		if err := validateURL(app.URL, allowHTTP); err != nil {
			return fmt.Errorf("%s.url: %w", path, err)
		}
		if app.FileName == "" {
			return fmt.Errorf("pole %s.fileName je u povolené aplikace povinné", path)
		}
		if !IsSafeInstallerFileName(app.FileName) {
			return fmt.Errorf("pole %s.fileName není bezpečný základní název souboru systému Windows", path)
		}
		switch app.InstallerType {
		case "auto", "exe", "msi":
		default:
			return fmt.Errorf("pole %s.installerType musí mít hodnotu auto, exe nebo msi", path)
		}
		isMSI := app.InstallerType == "msi" || (app.InstallerType == "auto" && strings.HasSuffix(strings.ToLower(app.FileName), ".msi"))
		if isMSI && !app.ShouldWait() {
			return fmt.Errorf("pole %s.wait musí mít u instalátoru MSI hodnotu true", path)
		}
	}
	return nil
}

// IsSafeInstallerFileName ověří, že hodnota představuje pouze bezpečný
// základní název souboru systému Windows bez cesty nebo datového proudu.
func IsSafeInstallerFileName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.TrimRight(name, " .") != name {
		return false
	}
	if strings.ContainsAny(name, `\/:*?"<>|`) {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	stem := name
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	switch strings.ToUpper(stem) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	}
	return true
}

func validateURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("URL má neplatný zápis")
	}
	if u.Host == "" {
		return errors.New("je vyžadována absolutní URL s názvem hostitele")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if allowHTTP {
			return nil
		}
		return errors.New("HTTP je zakázáno; použijte HTTPS nebo výslovně nastavte network.allowHttp")
	default:
		return fmt.Errorf("nepodporované schéma URL %q", u.Scheme)
	}
}

func validSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// EnabledApps vrátí pouze aplikace označené jako povolené.
func (c *Config) EnabledApps() []AppConfig {
	apps := make([]AppConfig, 0, len(c.Apps))
	for _, app := range c.Apps {
		if app.Enabled != nil && *app.Enabled {
			apps = append(apps, app)
		}
	}
	return apps
}

// ShouldWait určuje, zda se má čekat na ukončení instalátoru.
func (a AppConfig) ShouldWait() bool {
	return a.Wait == nil || *a.Wait
}

// HasValidSHA256 vrátí true, pokud SHA256 obsahuje úplný hexadecimální otisk.
// Chybějící nebo neplatný otisk je povolen, ověřování je však vypnuto.
func (a AppConfig) HasValidSHA256() bool {
	return validSHA256(a.SHA256)
}
