package config

import (
	"context"
	"strings"
	"testing"
)

func TestLoadValidEmbeddedConfig(t *testing.T) {
	cfg, err := Load([]byte(`{
		"productName":"Test",
		"remoteConfigUrl":"https://jandy.cz/apps.php?get=applist",
		"network":{"configTimeoutSeconds":15,"maxConfigBytes":4096}
	}`))
	if err != nil {
		t.Fatalf("Load() vrátil chybu: %v", err)
	}
	if cfg.ProductName != "Test" {
		t.Fatalf("ProductName = %q, očekáváno Test", cfg.ProductName)
	}
	if cfg.RemoteConfigURL != "https://jandy.cz/apps.php?get=applist" {
		t.Fatalf("RemoteConfigURL = %q", cfg.RemoteConfigURL)
	}
	if cfg.Network.ConfigTimeoutSeconds != 15 || cfg.Network.MaxConfigBytes != 4096 {
		t.Fatalf("neočekávaná síťová konfigurace: %#v", cfg.Network)
	}
}

func TestFetchRemoteAppsRejectsMissingURL(t *testing.T) {
	cfg, err := Load([]byte(`{}`))
	if err != nil {
		t.Fatalf("Load() vrátil chybu: %v", err)
	}
	if _, err := FetchRemoteApps(context.Background(), cfg); err == nil {
		t.Fatal("FetchRemoteApps() neočekávaně přijal prázdnou remoteConfigUrl")
	}
}

func TestLoadAcceptsEndpointWithoutJSONExtension(t *testing.T) {
	_, err := Load([]byte(`{"remoteConfigUrl":"https://jandy.cz/apps.php?get=applist"}`))
	if err != nil {
		t.Fatalf("Load() odmítl koncový bod PHP: %v", err)
	}
}

func TestFetchRemoteAppsRejectsRemoteHTTPByDefault(t *testing.T) {
	cfg, err := Load([]byte(`{"remoteConfigUrl":"http://jandy.cz/apps.php"}`))
	if err != nil {
		t.Fatalf("Load() vrátil chybu: %v", err)
	}
	if _, err := FetchRemoteApps(context.Background(), cfg); err == nil {
		t.Fatal("FetchRemoteApps() neočekávaně přijal HTTP")
	}
}

func TestLoadRemoteAppsValidWithSHA256(t *testing.T) {
	data := []byte(`{
		"apps":[{
			"id":1,
			"name":"Aplikace",
			"enabled":true,
			"url":"https://jandy.cz/app.exe",
			"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"fileName":"app.exe"
		}]
	}`)
	apps, err := LoadRemoteApps(data, NetworkConfig{})
	if err != nil {
		t.Fatalf("LoadRemoteApps() vrátil chybu: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("počet aplikací = %d, očekáváno 1", len(apps))
	}
	if apps[0].ID != 1 {
		t.Fatalf("ID aplikace = %d, očekáváno 1", apps[0].ID)
	}
	if !apps[0].ShouldWait() {
		t.Fatal("výchozí hodnota wait má být true")
	}
	if !apps[0].HasValidSHA256() {
		t.Fatal("platný SHA-256 nebyl rozpoznán")
	}
}

func TestLoadRemoteAppsAcceptsMissingOrInvalidSHA256(t *testing.T) {
	for _, sha := range []string{"", "neni-otisk"} {
		data := []byte(`{
			"apps":[{
				"id":1,
				"name":"Aplikace",
				"enabled":true,
				"url":"https://jandy.cz/app.exe",
				"sha256":"` + sha + `",
				"fileName":"app.exe"
			}]
		}`)
		apps, err := LoadRemoteApps(data, NetworkConfig{})
		if err != nil {
			t.Fatalf("LoadRemoteApps() se SHA-256 %q vrátil chybu: %v", sha, err)
		}
		if apps[0].HasValidSHA256() {
			t.Fatalf("SHA-256 %q měl být vyhodnocen jako neplatný", sha)
		}
	}
}

func TestLoadRemoteAppsRejectsEnabledHTTPByDefault(t *testing.T) {
	data := []byte(`{
		"apps":[{
			"id":1,
			"name":"Aplikace",
			"enabled":true,
			"url":"http://jandy.cz/app.exe",
			"fileName":"app.exe"
		}]
	}`)
	if _, err := LoadRemoteApps(data, NetworkConfig{}); err == nil {
		t.Fatal("LoadRemoteApps() neočekávaně přijal HTTP")
	}
}

func TestLoadRemoteAppsAllowsIncompleteDisabledTemplate(t *testing.T) {
	data := []byte(`{"apps":[{"id":1,"enabled":false}]}`)
	if _, err := LoadRemoteApps(data, NetworkConfig{}); err != nil {
		t.Fatalf("LoadRemoteApps() vrátil chybu: %v", err)
	}
}

func TestLoadRemoteAppsRequiresAppsProperty(t *testing.T) {
	if _, err := LoadRemoteApps([]byte(`{}`), NetworkConfig{}); err == nil {
		t.Fatal("LoadRemoteApps() přijal dokument bez pole apps")
	}
}

func TestLoadRemoteAppsRequiresEnabledProperty(t *testing.T) {
	data := []byte(`{"apps":[{"id":1}]}`)
	if _, err := LoadRemoteApps(data, NetworkConfig{}); err == nil {
		t.Fatal("LoadRemoteApps() přijal aplikaci bez pole enabled")
	}
}

func TestLoadRemoteAppsRejectsStringID(t *testing.T) {
	data := []byte(`{"apps":[{"id":"1","enabled":false}]}`)
	if _, err := LoadRemoteApps(data, NetworkConfig{}); err == nil {
		t.Fatal("LoadRemoteApps() neočekávaně přijal textové ID aplikace")
	}
}

func TestLoadRemoteAppsRejectsMissingOrNonPositiveID(t *testing.T) {
	tests := []string{
		`{"apps":[{"enabled":false}]}`,
		`{"apps":[{"id":0,"enabled":false}]}`,
		`{"apps":[{"id":-1,"enabled":false}]}`,
	}
	for _, data := range tests {
		if _, err := LoadRemoteApps([]byte(data), NetworkConfig{}); err == nil {
			t.Fatalf("LoadRemoteApps() neočekávaně přijal neplatné ID: %s", data)
		}
	}
}

func TestLoadRemoteAppsRejectsDuplicateIntegerID(t *testing.T) {
	data := []byte(`{"apps":[{"id":1,"enabled":false},{"id":1,"enabled":false}]}`)
	if _, err := LoadRemoteApps(data, NetworkConfig{}); err == nil {
		t.Fatal("LoadRemoteApps() neočekávaně přijal duplicitní celočíselné ID")
	}
}

func TestLoadRemoteAppsAcceptsEmptyList(t *testing.T) {
	apps, err := LoadRemoteApps([]byte(`{"apps":[]}`), NetworkConfig{})
	if err != nil {
		t.Fatalf("LoadRemoteApps() vrátil chybu: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("počet aplikací = %d, očekáváno 0", len(apps))
	}
}

func TestLoadRemoteAppsAcceptsUTF8BOM(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"apps":[]}`)...)
	if _, err := LoadRemoteApps(data, NetworkConfig{}); err != nil {
		t.Fatalf("LoadRemoteApps() odmítl UTF-8 BOM: %v", err)
	}
}

func TestLoadRemoteAppsRejectsUnknownField(t *testing.T) {
	_, err := LoadRemoteApps([]byte(`{"apps":[],"neocekavane":true}`), NetworkConfig{})
	if err == nil || !strings.Contains(err.Error(), "neocekavane") {
		t.Fatalf("očekávána chyba neznámého pole, získáno: %v", err)
	}
}

func TestDefaultsUseESSENSNames(t *testing.T) {
	cfg, err := Load([]byte(`{"remoteConfigUrl":"https://jandy.cz/apps.php"}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProductName != "ESSENS Provisioner" {
		t.Fatalf("ProductName = %q", cfg.ProductName)
	}
	if cfg.CacheDirectory != `${ProgramData}\ESSProvisioner\cache` {
		t.Fatalf("CacheDirectory = %q", cfg.CacheDirectory)
	}
	if cfg.Network.ConfigTimeoutSeconds != 30 {
		t.Fatalf("ConfigTimeoutSeconds = %d", cfg.Network.ConfigTimeoutSeconds)
	}
	if cfg.Network.MaxConfigBytes != 1024*1024 {
		t.Fatalf("MaxConfigBytes = %d", cfg.Network.MaxConfigBytes)
	}
}

func TestIsSafeInstallerFileName(t *testing.T) {
	valid := []string{"installer.exe", "Instalace dodavatele 1.2.msi", "工具.exe"}
	for _, name := range valid {
		if !IsSafeInstallerFileName(name) {
			t.Errorf("%q má být platný název", name)
		}
	}
	invalid := []string{"../evil.exe", `..\evil.exe`, "file.exe:stream", "CON.exe", "trailing.exe.", "a/b.exe"}
	for _, name := range invalid {
		if IsSafeInstallerFileName(name) {
			t.Errorf("%q má být neplatný název", name)
		}
	}
}

func TestEmbeddedConfigLoads(t *testing.T) {
	if _, err := LoadEmbedded(); err != nil {
		t.Fatalf("LoadEmbedded() vrátil chybu: %v", err)
	}
}

func TestLoadRemoteAppsRejectsNonWaitingMSI(t *testing.T) {
	data := []byte(`{
		"apps":[{
			"id":1,
			"name":"Aplikace",
			"enabled":true,
			"url":"https://jandy.cz/app.msi",
			"fileName":"app.msi",
			"installerType":"msi",
			"wait":false
		}]
	}`)
	if _, err := LoadRemoteApps(data, NetworkConfig{}); err == nil {
		t.Fatal("LoadRemoteApps() neočekávaně přijal wait=false pro MSI")
	}
}
