package install

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	genericRead         = 0x80000000
	fileShareRead       = 0x00000001
	openExisting        = 3
	fileAttributeNormal = 0x00000080
	invalidHandleValue  = ^uintptr(0)
)

var (
	kernel32CreateFile       = syscall.NewLazyDLL("kernel32.dll")
	procCreateFileWForVerify = kernel32CreateFile.NewProc("CreateFileW")
)

// lockAndVerifyInstaller otevře instalátor bez sdílení pro zápis nebo odstranění
// a ponechá popisovač otevřený až do dokončení spuštění. Pokud expectedHex
// obsahuje platný SHA-256, ověří se před vytvořením procesu přesně tento soubor.
func lockAndVerifyInstaller(path, expectedHex string) (io.Closer, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, _, callErr := procCreateFileWForVerify.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		genericRead,
		fileShareRead,
		0,
		openExisting,
		fileAttributeNormal,
		0,
	)
	if handle == invalidHandleValue {
		if errno, ok := callErr.(syscall.Errno); ok && errno != 0 {
			return nil, errno
		}
		return nil, syscall.EINVAL
	}
	file := os.NewFile(handle, path)
	if file == nil {
		syscall.CloseHandle(syscall.Handle(handle))
		return nil, fmt.Errorf("vytvoření souboru Go z popisovače systému Windows")
	}
	if validSHA256(expectedHex) {
		if err := verifyOpenFile(file, expectedHex); err != nil {
			file.Close()
			return nil, err
		}
	}
	return file, nil
}

func verifyOpenFile(file *os.File, expectedHex string) error {
	expected, _ := hex.DecodeString(strings.TrimSpace(expectedHex))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(h.Sum(nil), expected) != 1 {
		return fmt.Errorf("instalátor se po ověření stažení změnil")
	}
	_, err := file.Seek(0, io.SeekStart)
	return err
}

func validSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
