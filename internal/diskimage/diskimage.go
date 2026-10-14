// Package diskimage připojuje obrazy optických disků ISO a IMG nativními
// prostředky Windows (virtdisk.dll), bez programů třetích stran a bez
// spouštění příkazového řádku.
//
// Mount připojí obraz pouze pro čtení a zjistí cestu ke svazku, například
// `E:\`. Unmount obraz odpojí jen tehdy, když ho připojil právě WinFly; obraz
// připojený uživatelem nebo jinou aplikací zůstane připojený.
//
// Jak soubory spolupracují:
//
//   - diskimage.go        Mounter, Image a pravidla vlastnictví připojení
//   - format.go           rozpoznání obrazu ISO 9660 nebo UDF podle obsahu
//   - autorun.go          ověření instalátoru podle autorun.inf (nic nespouští)
//   - cmdline.go          rozdělení příkazu z autorun.inf na program a parametry
//   - system_windows.go   napojení na Windows: připojení, odpojení, svazek
//   - virtdisk_windows.go volání knihovny virtdisk.dll
//   - volume_windows.go   vyhledání svazku a písmena jednotky
//   - system_other.go     náhrada pro jiné systémy než Windows
package diskimage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

var (
	// ErrImageNotFound znamená, že soubor obrazu neexistuje.
	ErrImageNotFound = errors.New("soubor obrazu disku neexistuje")
	// ErrAccessDenied znamená, že k připojení chybí oprávnění.
	ErrAccessDenied = errors.New("k připojení obrazu disku chybí oprávnění")
	// ErrNoVolume znamená, že Windows po připojení nezpřístupnil žádný svazek.
	ErrNoVolume = errors.New("obraz disku byl připojen, ale Windows k němu nezpřístupnil svazek ani písmeno jednotky")

	// errVolumeNotReady vrací system.volumeOf, dokud Windows svazek ještě
	// nevytvořil; Mount to zkouší znovu.
	errVolumeNotReady = errors.New("svazek zatím není připraven")
)

// diskHandle je popisovač otevřeného souboru obrazu.
type diskHandle uintptr

// system je část operačního systému, kterou Mounter potřebuje. Skutečná
// implementace je v system_windows.go, testy používají náhradu.
type system interface {
	// openImage otevře obraz pro připojení pouze pro čtení.
	openImage(path string) (diskHandle, error)
	// attachedDevice vrátí cestu k zařízení (např. `\\.\CDROM1`), pokud je
	// obraz již připojen, ať už kýmkoli.
	attachedDevice(h diskHandle) (device string, attached bool)
	// attach připojí obraz pouze pro čtení. Připojení zanikne nejpozději se
	// zavřením popisovače, i když WinFly neočekávaně skončí.
	attach(h diskHandle) error
	detach(h diskHandle) error
	closeImage(h diskHandle) error
	// volumeOf vrátí cestu ke svazku zařízení, například `E:\`, nebo
	// errVolumeNotReady, dokud svazek neexistuje.
	volumeOf(device string) (string, error)
}

// Mounter připojuje a odpojuje obrazy disků.
type Mounter struct {
	sys          system
	volumeWait   time.Duration // jak dlouho čekat, než Windows vytvoří svazek
	pollInterval time.Duration
}

// NewMounter vrátí Mounter, který používá nativní funkce Windows.
func NewMounter() *Mounter {
	return &Mounter{
		sys:          newSystem(),
		volumeWait:   30 * time.Second,
		pollInterval: 250 * time.Millisecond,
	}
}

// Image je připojený obraz disku.
type Image struct {
	// ImagePath je absolutní cesta k souboru obrazu.
	ImagePath string
	// MountPath je kořen připojeného svazku, například `E:\`. Pokud Windows
	// nepřiřadil písmeno jednotky, jde o cestu `\\?\Volume{…}\`.
	MountPath string
	// Owned je true, pokud obraz připojil tento Mounter. Jen takový obraz
	// Unmount skutečně odpojí.
	Owned bool

	sys    system
	handle diskHandle
}

// Mount ověří formát obrazu, připojí ho pouze pro čtení a počká na jeho svazek.
// Pokud je obraz již připojen jinou aplikací, použije se existující připojení
// a výsledek má Owned=false.
func (m *Mounter) Mount(ctx context.Context, imagePath string) (*Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := checkImageFile(imagePath)
	if err != nil {
		return nil, err
	}

	handle, err := m.sys.openImage(path)
	if err != nil {
		return nil, fmt.Errorf("otevření obrazu disku %q: %w", path, err)
	}
	img := &Image{
		ImagePath: path,
		sys:       m.sys,
		handle:    handle,
	}

	device, attached := m.sys.attachedDevice(handle)
	if !attached {
		device, err = img.attach()
		if err != nil {
			_ = img.Unmount()
			return nil, err
		}
	}

	img.MountPath, err = m.waitForVolume(ctx, device)
	if err != nil {
		_ = img.Unmount()
		return nil, fmt.Errorf("připojení obrazu disku %q: %w", path, err)
	}
	return img, nil
}

// Unmount odpojí obraz, pokud ho připojil tento Mounter, a uvolní popisovač.
// Obraz připojený jinou aplikací zůstane připojený. Opakované volání nic nedělá.
func (img *Image) Unmount() error {
	if img.handle == 0 {
		return nil
	}
	var detachErr error
	if img.Owned {
		detachErr = img.sys.detach(img.handle)
	}
	closeErr := img.sys.closeImage(img.handle)
	img.handle = 0

	if detachErr != nil {
		return fmt.Errorf("odpojení obrazu disku %q: %w", img.ImagePath, detachErr)
	}
	if closeErr != nil {
		return fmt.Errorf("uzavření obrazu disku %q: %w", img.ImagePath, closeErr)
	}
	return nil
}

// attach připojí obraz a označí ho jako vlastní.
func (img *Image) attach() (string, error) {
	if err := img.sys.attach(img.handle); err != nil {
		return "", fmt.Errorf("připojení obrazu disku %q: %w", img.ImagePath, err)
	}
	img.Owned = true

	device, attached := img.sys.attachedDevice(img.handle)
	if !attached {
		return "", fmt.Errorf("připojení obrazu disku %q: %w", img.ImagePath, ErrNoVolume)
	}
	return device, nil
}

// waitForVolume čeká, než Windows k připojenému zařízení vytvoří svazek.
func (m *Mounter) waitForVolume(ctx context.Context, device string) (string, error) {
	deadline := time.Now().Add(m.volumeWait)
	for {
		volume, err := m.sys.volumeOf(device)
		if err == nil {
			return volume, nil
		}
		if !errors.Is(err, errVolumeNotReady) {
			return "", err
		}
		if time.Now().After(deadline) {
			return "", ErrNoVolume
		}

		timer := time.NewTimer(m.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

// checkImageFile převede cestu na absolutní a ověří, že soubor existuje a má
// podporovaný formát.
func checkImageFile(imagePath string) (string, error) {
	path, err := filepath.Abs(imagePath)
	if err != nil {
		return "", fmt.Errorf("převod cesty k obrazu disku %q na absolutní: %w", imagePath, err)
	}

	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("%w: %q", ErrImageNotFound, path)
	case errors.Is(err, fs.ErrPermission):
		return "", fmt.Errorf("%w: %q: %w", ErrAccessDenied, path, err)
	case err != nil:
		return "", fmt.Errorf("zjištění informací o obrazu disku %q: %w", path, err)
	case info.IsDir():
		return "", fmt.Errorf("cesta k obrazu disku %q odkazuje na adresář", path)
	}

	err = CheckFile(path)
	switch {
	case errors.Is(err, fs.ErrPermission):
		return "", fmt.Errorf("%w: %q: %w", ErrAccessDenied, path, err)
	case err != nil:
		return "", fmt.Errorf("kontrola obrazu disku %q: %w", path, err)
	}
	return path, nil
}
