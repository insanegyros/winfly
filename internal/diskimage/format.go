package diskimage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrUnsupportedFormat znamená, že soubor není obraz optického disku, který
// umí Windows připojit (ISO 9660 nebo UDF).
var ErrUnsupportedFormat = errors.New("nepodporovaný formát obrazu disku")

const (
	// Obrazy ISO 9660 i UDF mají sektory po 2048 bajtech a první popisovač
	// svazku v sektoru 16.
	sectorSize             = 2048
	firstDescriptorOffset  = 16 * sectorSize
	maxDescriptorsToScan   = 32
	descriptorHeaderLength = 6 // typ (1 bajt) + identifikátor (5 bajtů)

	// Surové obrazy CD (např. BIN/IMG z CloneCD) mají sektory po 2352 bajtech.
	rawSectorSize = 2352
)

// rawSectorSync je synchronizační vzor na začátku každého surového sektoru CD.
var rawSectorSync = []byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00}

// CheckFile ověří, že soubor na cestě path je obraz optického disku, který lze
// připojit nativními prostředky Windows. Přípona souboru nehraje roli.
func CheckFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return checkFormat(f)
}

// checkFormat hledá popisovače svazku ISO 9660 (CD001) nebo UDF (NSR02,
// NSR03). Pokud žádný nenajde, vrátí chybu s co nejpřesnějším popisem.
func checkFormat(r io.ReaderAt) error {
	for i := range maxDescriptorsToScan {
		header := make([]byte, descriptorHeaderLength)
		if _, err := r.ReadAt(header, int64(firstDescriptorOffset+i*sectorSize)); err != nil {
			break
		}
		switch string(header[1:]) {
		case "CD001", "NSR02", "NSR03":
			return nil
		case "BEA01", "TEA01", "BOOT2", "CDW02":
			// Další části rozpoznávací sekvence UDF; pokračuje se dalším sektorem.
			continue
		}
		break
	}
	return fmt.Errorf("%w: %s", ErrUnsupportedFormat, unsupportedReason(r))
}

// unsupportedReason popíše, o jaký nepodporovaný soubor se nejspíš jedná.
func unsupportedReason(r io.ReaderAt) string {
	start := make([]byte, 512)
	n, _ := r.ReadAt(start, 0)
	start = start[:n]

	rawHeader := make([]byte, descriptorHeaderLength)
	rawDescriptorOffset := int64(16*rawSectorSize + 16) // 16 bajtů hlavičky surového sektoru
	_, rawErr := r.ReadAt(rawHeader, rawDescriptorOffset)
	isRawCD := bytes.HasPrefix(start, rawSectorSync) ||
		(rawErr == nil && string(rawHeader[1:]) == "CD001")
	hasBootSignature := len(start) == 512 && start[510] == 0x55 && start[511] == 0xAA

	switch {
	case n < 512:
		return "soubor je příliš malý na obraz disku"
	case isRawCD:
		return "jde o surový obraz CD se sektory po 2352 bajtech (např. BIN/IMG z CloneCD), který Windows neumí připojit"
	case hasBootSignature:
		return "jde o obraz pevného disku, USB nebo SD karty, nikoli o obraz optického disku ISO 9660 nebo UDF"
	default:
		return "soubor neobsahuje systém souborů ISO 9660 ani UDF"
	}
}
