package systeminfo

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

var directXValuePattern = regexp.MustCompile(`(?i)\bDirectX\s*[0-9]+(?:\.[0-9]+)?\b`)
var xmlUTF16EncodingPattern = regexp.MustCompile(`(?i)encoding\s*=\s*["']utf-16["']`)

func directXVersion(parent context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()

	tempFile, err := os.CreateTemp("", "essprovisioner-dxdiag-*.xml")
	if err != nil {
		return "", err
	}
	path := tempFile.Name()
	if closeErr := tempFile.Close(); closeErr != nil {
		_ = os.Remove(path)
		return "", closeErr
	}
	_ = os.Remove(path)
	defer os.Remove(path)

	program := "dxdiag.exe"
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		program = filepath.Join(systemRoot, "System32", "dxdiag.exe")
	}
	cmd := exec.CommandContext(ctx, program, "/whql:off", "/x", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("dxdiag.exe překročil časový limit 20 sekund")
		}
		message := strings.TrimSpace(string(output))
		if message != "" {
			return "", fmt.Errorf("dxdiag.exe selhal: %w: %s", runErr, message)
		}
		return "", fmt.Errorf("dxdiag.exe selhal: %w", runErr)
	}

	const maxDXDiagBytes = 16 * 1024 * 1024
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("otevření výstupu dxdiag.exe: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxDXDiagBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("načtení výstupu dxdiag.exe: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("uzavření výstupu dxdiag.exe: %w", closeErr)
	}
	if len(data) > maxDXDiagBytes {
		return "", errors.New("výstup dxdiag.exe překročil limit 16 MB")
	}
	data, err = normalizeXMLText(data)
	if err != nil {
		return "", err
	}
	return parseDirectXVersionXML(data)
}

func normalizeXMLText(data []byte) ([]byte, error) {
	if len(data) >= 2 && data[0] == 0xff && data[1] == 0xfe {
		decoded, err := decodeUTF16(data[2:], binary.LittleEndian)
		return normalizeXMLDeclaration(decoded), err
	}
	if len(data) >= 2 && data[0] == 0xfe && data[1] == 0xff {
		decoded, err := decodeUTF16(data[2:], binary.BigEndian)
		return normalizeXMLDeclaration(decoded), err
	}
	if looksLikeUTF16LE(data) {
		decoded, err := decodeUTF16(data, binary.LittleEndian)
		return normalizeXMLDeclaration(decoded), err
	}
	if looksLikeUTF16BE(data) {
		decoded, err := decodeUTF16(data, binary.BigEndian)
		return normalizeXMLDeclaration(decoded), err
	}
	return normalizeXMLDeclaration(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})), nil
}

func looksLikeUTF16LE(data []byte) bool {
	return len(data) >= 8 && data[0] == '<' && data[1] == 0 && data[2] == '?' && data[3] == 0
}

func looksLikeUTF16BE(data []byte) bool {
	return len(data) >= 8 && data[0] == 0 && data[1] == '<' && data[2] == 0 && data[3] == '?'
}

func normalizeXMLDeclaration(data []byte) []byte {
	return xmlUTF16EncodingPattern.ReplaceAll(data, []byte(`encoding="utf-8"`))
}

func decodeUTF16(data []byte, order binary.ByteOrder) ([]byte, error) {
	if len(data)%2 != 0 {
		return nil, errors.New("výstup dxdiag.exe obsahuje neplatný UTF-16 text")
	}
	units := make([]uint16, len(data)/2)
	for index := range units {
		units[index] = order.Uint16(data[index*2 : index*2+2])
	}
	return []byte(string(utf16.Decode(units))), nil
}

func parseDirectXVersionXML(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("zpracování XML výstupu dxdiag.exe: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(start.Name.Local)
		if name != "directxversion" && !(strings.Contains(name, "directx") && strings.Contains(name, "version")) {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err != nil {
			return "", fmt.Errorf("zpracování položky verze DirectX: %w", err)
		}
		if match := directXValuePattern.FindString(strings.TrimSpace(value)); match != "" {
			return normalizeDirectXLabel(match), nil
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}

	if match := directXValuePattern.Find(data); len(match) > 0 {
		return normalizeDirectXLabel(string(match)), nil
	}
	return "", errors.New("dxdiag.exe nevrátil rozpoznatelnou verzi DirectX")
}

func normalizeDirectXLabel(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "directx") {
		suffix := strings.TrimSpace(value[len("DirectX"):])
		if suffix == "" {
			return "DirectX"
		}
		return "DirectX " + suffix
	}
	return value
}
