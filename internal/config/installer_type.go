package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Hodnoty pole installerType ve vzdálené konfiguraci.
const (
	InstallerTypeAuto = "auto" // typ se určí podle přípony názvu souboru
	InstallerTypeEXE  = "exe"
	InstallerTypeMSI  = "msi"
	InstallerTypeISO  = "iso" // obraz disku ISO 9660 nebo UDF
	InstallerTypeIMG  = "img" // obraz disku se systémem souborů ISO 9660 nebo UDF
)

// ResolveInstallerType vrátí konkrétní typ instalátoru (exe, msi, iso nebo img).
// U typu auto nebo prázdné hodnoty se typ určí z přípony fileName bez ohledu
// na velikost písmen.
func ResolveInstallerType(installerType, fileName string) (string, error) {
	kind := strings.ToLower(strings.TrimSpace(installerType))
	if kind == "" || kind == InstallerTypeAuto {
		return installerTypeFromExtension(fileName)
	}
	if !isConcreteInstallerType(kind) {
		return "", fmt.Errorf("nepodporovaný typ instalátoru %q; povolené hodnoty jsou auto, exe, msi, iso a img", kind)
	}
	return kind, nil
}

// IsDiskImageType vrátí true pro typy, které se před instalací připojují
// jako obraz disku.
func IsDiskImageType(kind string) bool {
	return kind == InstallerTypeISO || kind == InstallerTypeIMG
}

// requiresWait vrátí true pro typy, u kterých musí WinFly čekat na dokončení
// instalátoru: MSI kvůli uzamčení balíčku, obraz disku kvůli jeho odpojení.
func requiresWait(kind string) bool {
	return kind == InstallerTypeMSI || IsDiskImageType(kind)
}

func isKnownInstallerType(kind string) bool {
	return kind == InstallerTypeAuto || isConcreteInstallerType(kind)
}

func isConcreteInstallerType(kind string) bool {
	switch kind {
	case InstallerTypeEXE, InstallerTypeMSI, InstallerTypeISO, InstallerTypeIMG:
		return true
	default:
		return false
	}
}

func installerTypeFromExtension(fileName string) (string, error) {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".exe":
		return InstallerTypeEXE, nil
	case ".msi":
		return InstallerTypeMSI, nil
	case ".iso":
		return InstallerTypeISO, nil
	case ".img":
		return InstallerTypeIMG, nil
	default:
		return "", fmt.Errorf("typ instalátoru nelze určit z názvu %q; podporované přípony jsou .exe, .msi, .iso a .img", fileName)
	}
}
