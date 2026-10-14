package diskimage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isoImage vrátí nejmenší obsah, který checkFormat přijme jako ISO 9660.
func isoImage() []byte {
	data := make([]byte, firstDescriptorOffset+2*sectorSize)
	copy(data[firstDescriptorOffset:], "\x01CD001\x01")
	copy(data[firstDescriptorOffset+sectorSize:], "\xffCD001\x01")
	return data
}

// udfImage vrátí rozpoznávací sekvenci čistého UDF bez ISO 9660.
func udfImage() []byte {
	data := make([]byte, firstDescriptorOffset+3*sectorSize)
	copy(data[firstDescriptorOffset:], "\x00BEA01\x01")
	copy(data[firstDescriptorOffset+sectorSize:], "\x00NSR02\x01")
	copy(data[firstDescriptorOffset+2*sectorSize:], "\x00TEA01\x01")
	return data
}

func hardDiskImage() []byte {
	data := make([]byte, 64*1024)
	data[510], data[511] = 0x55, 0xAA
	return data
}

func rawCDImage() []byte {
	data := make([]byte, 20*rawSectorSize)
	copy(data, rawSectorSync)
	copy(data[16*rawSectorSize+16:], "\x01CD001\x01")
	return data
}

func writeImage(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckFileAcceptsOpticalImages(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "instalace.iso", data: isoImage()},
		{name: "instalace.img", data: isoImage()},
		{name: "udf.IMG", data: udfImage()},
		{name: "Program Files ÚDF žluťoučký.iso", data: udfImage()},
	}
	for _, tt := range tests {
		if err := CheckFile(writeImage(t, tt.name, tt.data)); err != nil {
			t.Errorf("%s: CheckFile() vrátil chybu: %v", tt.name, err)
		}
	}
}

func TestCheckFileRejectsUnsupportedImages(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		wantInError string
	}{
		{name: "usb.img", data: hardDiskImage(), wantInError: "pevného disku"},
		{name: "clonecd.img", data: rawCDImage(), wantInError: "2352"},
		{name: "tiny.img", data: []byte("abc"), wantInError: "příliš malý"},
		{name: "random.img", data: make([]byte, 64*1024), wantInError: "ISO 9660 ani UDF"},
	}
	for _, tt := range tests {
		err := CheckFile(writeImage(t, tt.name, tt.data))
		if !errors.Is(err, ErrUnsupportedFormat) {
			t.Errorf("%s: CheckFile() = %v, očekávána ErrUnsupportedFormat", tt.name, err)
			continue
		}
		if !strings.Contains(err.Error(), tt.wantInError) {
			t.Errorf("%s: chyba %q neobsahuje %q", tt.name, err, tt.wantInError)
		}
	}
}
