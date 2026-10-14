package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestResolveInstallerType(t *testing.T) {
	tests := []struct {
		installerType string
		fileName      string
		want          string
	}{
		{installerType: "auto", fileName: "setup.exe", want: InstallerTypeEXE},
		{installerType: "auto", fileName: "setup.msi", want: InstallerTypeMSI},
		{installerType: "auto", fileName: "office.iso", want: InstallerTypeISO},
		{installerType: "auto", fileName: "office.img", want: InstallerTypeIMG},
		{installerType: "auto", fileName: "OFFICE.ISO", want: InstallerTypeISO},
		{installerType: "auto", fileName: "Office.Img", want: InstallerTypeIMG},
		{installerType: "", fileName: "setup.EXE", want: InstallerTypeEXE},
		{installerType: "AUTO", fileName: `C:\cache\setup.Msi`, want: InstallerTypeMSI},
		{installerType: "exe", fileName: "setup.bin", want: InstallerTypeEXE},
		{installerType: "msi", fileName: "setup.bin", want: InstallerTypeMSI},
		{installerType: " ISO ", fileName: "image.bin", want: InstallerTypeISO},
		{installerType: "Img", fileName: "image.bin", want: InstallerTypeIMG},
	}
	for _, tt := range tests {
		got, err := ResolveInstallerType(tt.installerType, tt.fileName)
		if err != nil {
			t.Errorf("ResolveInstallerType(%q, %q) vrátil chybu: %v", tt.installerType, tt.fileName, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ResolveInstallerType(%q, %q) = %q, očekáváno %q", tt.installerType, tt.fileName, got, tt.want)
		}
	}
}

func TestResolveInstallerTypeRejectsUnsupported(t *testing.T) {
	tests := []struct {
		installerType string
		fileName      string
		wantInError   string
	}{
		{installerType: "zip", fileName: "app.zip", wantInError: `"zip"`},
		{installerType: "vhd", fileName: "app.vhd", wantInError: "auto, exe, msi, iso a img"},
		{installerType: "auto", fileName: "app.bin", wantInError: ".exe, .msi, .iso a .img"},
		{installerType: "auto", fileName: "app", wantInError: `"app"`},
	}
	for _, tt := range tests {
		_, err := ResolveInstallerType(tt.installerType, tt.fileName)
		if err == nil {
			t.Errorf("ResolveInstallerType(%q, %q) neočekávaně uspěl", tt.installerType, tt.fileName)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantInError) {
			t.Errorf("chyba %q neobsahuje %q", err, tt.wantInError)
		}
	}
}

func TestIsDiskImageType(t *testing.T) {
	for _, kind := range []string{InstallerTypeISO, InstallerTypeIMG} {
		if !IsDiskImageType(kind) {
			t.Errorf("IsDiskImageType(%q) = false", kind)
		}
	}
	for _, kind := range []string{InstallerTypeAuto, InstallerTypeEXE, InstallerTypeMSI, ""} {
		if IsDiskImageType(kind) {
			t.Errorf("IsDiskImageType(%q) = true", kind)
		}
	}
}

// remoteAppJSON vrátí vzdálený seznam s jednou povolenou aplikací.
func remoteAppJSON(fileName, installerType string, wait bool) []byte {
	return []byte(fmt.Sprintf(`{
		"apps":[{
			"id":1,
			"name":"Aplikace",
			"enabled":true,
			"url":"https://jandy.cz/%[1]s",
			"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"fileName":"%[1]s",
			"installerType":"%[2]s",
			"arguments":["/quiet"],
			"wait":%[3]t,
			"successExitCodes":[0,3010]
		}]
	}`, fileName, installerType, wait))
}

func TestLoadRemoteAppsAcceptsDiskImages(t *testing.T) {
	tests := []struct {
		fileName      string
		installerType string
	}{
		{fileName: "office.iso", installerType: "iso"},
		{fileName: "office.img", installerType: "img"},
		{fileName: "OFFICE.ISO", installerType: "auto"},
		{fileName: "office.Img", installerType: "auto"},
		{fileName: "office.iso", installerType: "ISO"},
	}
	for _, tt := range tests {
		apps, err := LoadRemoteApps(remoteAppJSON(tt.fileName, tt.installerType, true), NetworkConfig{})
		if err != nil {
			t.Errorf("%s (%s): LoadRemoteApps() vrátil chybu: %v", tt.fileName, tt.installerType, err)
			continue
		}
		app := apps[0]
		if !app.HasValidSHA256() || !app.ShouldWait() {
			t.Errorf("%s: SHA-256 nebo wait se nenačetly", tt.fileName)
		}
		if len(app.Arguments) != 1 || len(app.SuccessExitCodes) != 2 {
			t.Errorf("%s: parametry nebo návratové kódy se nenačetly: %#v", tt.fileName, app)
		}
	}
}

func TestLoadRemoteAppsRequiresWaitForDiskImages(t *testing.T) {
	tests := []struct {
		fileName      string
		installerType string
	}{
		{fileName: "office.iso", installerType: "iso"},
		{fileName: "office.img", installerType: "img"},
		{fileName: "office.ISO", installerType: "auto"},
		{fileName: "office.img", installerType: "auto"},
	}
	for _, tt := range tests {
		_, err := LoadRemoteApps(remoteAppJSON(tt.fileName, tt.installerType, false), NetworkConfig{})
		if err == nil {
			t.Errorf("%s (%s): LoadRemoteApps() neočekávaně přijal wait=false", tt.fileName, tt.installerType)
			continue
		}
		if !strings.Contains(err.Error(), "wait") {
			t.Errorf("chyba %q nezmiňuje pole wait", err)
		}
	}
}

func TestLoadRemoteAppsRejectsUnsupportedInstallerType(t *testing.T) {
	_, err := LoadRemoteApps(remoteAppJSON("app.zip", "zip", true), NetworkConfig{})
	if err == nil {
		t.Fatal("LoadRemoteApps() neočekávaně přijal installerType zip")
	}
	if !strings.Contains(err.Error(), "auto, exe, msi, iso nebo img") {
		t.Fatalf("chyba %q neuvádí povolené hodnoty", err)
	}
}

// Starší seznamy mohou obsahovat typ auto s neznámou příponou (panel vytváří
// například název app.bin). Takový seznam se musí dál načíst.
func TestLoadRemoteAppsKeepsAutoWithUnknownExtension(t *testing.T) {
	if _, err := LoadRemoteApps(remoteAppJSON("app.bin", "auto", false), NetworkConfig{}); err != nil {
		t.Fatalf("LoadRemoteApps() odmítl starší položku: %v", err)
	}
}

func TestLoadRemoteAppsKeepsEXEAndMSIRules(t *testing.T) {
	accepted := []struct {
		fileName      string
		installerType string
		wait          bool
	}{
		{fileName: "app.exe", installerType: "auto", wait: false},
		{fileName: "app.exe", installerType: "exe", wait: true},
		{fileName: "app.msi", installerType: "auto", wait: true},
		{fileName: "app.msi", installerType: "msi", wait: true},
	}
	for _, tt := range accepted {
		if _, err := LoadRemoteApps(remoteAppJSON(tt.fileName, tt.installerType, tt.wait), NetworkConfig{}); err != nil {
			t.Errorf("%s (%s, wait=%t): neočekávaná chyba: %v", tt.fileName, tt.installerType, tt.wait, err)
		}
	}
	if _, err := LoadRemoteApps(remoteAppJSON("app.MSI", "auto", false), NetworkConfig{}); err == nil {
		t.Error("LoadRemoteApps() neočekávaně přijal MSI s wait=false")
	}
}
