package diskimage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// fakeSystem nahrazuje Windows v testech a zaznamenává volání.
type fakeSystem struct {
	alreadyAttached bool   // obraz připojil uživatel nebo jiná aplikace
	openErr         error  // chyba z openImage
	attachErr       error  // chyba z attach
	volume          string // výsledek volumeOf; prázdný znamená „svazek není připraven“

	attached   bool
	openedPath string
	calls      []string
}

func (f *fakeSystem) openImage(path string) (diskHandle, error) {
	f.calls = append(f.calls, "open")
	f.openedPath = path
	if f.openErr != nil {
		return 0, f.openErr
	}
	return 42, nil
}

func (f *fakeSystem) attachedDevice(diskHandle) (string, bool) {
	if f.alreadyAttached || f.attached {
		return `\\.\CDROM1`, true
	}
	return "", false
}

func (f *fakeSystem) attach(diskHandle) error {
	f.calls = append(f.calls, "attach")
	if f.attachErr != nil {
		return f.attachErr
	}
	f.attached = true
	return nil
}

func (f *fakeSystem) detach(diskHandle) error {
	f.calls = append(f.calls, "detach")
	f.attached = false
	return nil
}

func (f *fakeSystem) closeImage(diskHandle) error {
	f.calls = append(f.calls, "close")
	return nil
}

func (f *fakeSystem) volumeOf(device string) (string, error) {
	if f.volume == "" {
		return "", errVolumeNotReady
	}
	return f.volume, nil
}

func (f *fakeSystem) called(name string) bool {
	for _, call := range f.calls {
		if call == name {
			return true
		}
	}
	return false
}

func newTestMounter(sys *fakeSystem) *Mounter {
	return &Mounter{
		sys:          sys,
		volumeWait:   50 * time.Millisecond,
		pollInterval: time.Millisecond,
	}
}

func TestMountAndUnmountOwnedImage(t *testing.T) {
	sys := &fakeSystem{volume: `E:\`}
	path := writeImage(t, "Office 2024 žluťoučký kůň.iso", isoImage())

	img, err := newTestMounter(sys).Mount(context.Background(), path)
	if err != nil {
		t.Fatalf("Mount() vrátil chybu: %v", err)
	}
	if img.MountPath != `E:\` || !img.Owned || img.ImagePath != path {
		t.Fatalf("neočekávaný výsledek: %+v", img)
	}
	if sys.openedPath != path {
		t.Fatalf("otevřená cesta = %q, očekáváno %q", sys.openedPath, path)
	}

	if err := img.Unmount(); err != nil {
		t.Fatalf("Unmount() vrátil chybu: %v", err)
	}
	if sys.attached {
		t.Fatal("obraz zůstal po Unmount() připojený")
	}
	if err := img.Unmount(); err != nil {
		t.Fatalf("opakovaný Unmount() vrátil chybu: %v", err)
	}
	want := fmt.Sprint([]string{"open", "attach", "detach", "close"})
	if got := fmt.Sprint(sys.calls); got != want {
		t.Fatalf("volání = %s, očekáváno %s", got, want)
	}
}

func TestMountUsesForeignMountWithoutDetaching(t *testing.T) {
	sys := &fakeSystem{alreadyAttached: true, volume: `F:\`}
	img, err := newTestMounter(sys).Mount(context.Background(), writeImage(t, "user.iso", isoImage()))
	if err != nil {
		t.Fatalf("Mount() vrátil chybu: %v", err)
	}
	if img.Owned || img.MountPath != `F:\` {
		t.Fatalf("neočekávaný výsledek: %+v", img)
	}
	if err := img.Unmount(); err != nil {
		t.Fatal(err)
	}
	if sys.called("attach") || sys.called("detach") {
		t.Fatalf("cizí obraz byl připojen nebo odpojen: %v", sys.calls)
	}
	if !sys.called("close") {
		t.Fatal("popisovač obrazu nebyl uzavřen")
	}
}

func TestMountRejectsUnsupportedImageBeforeOpening(t *testing.T) {
	sys := &fakeSystem{volume: `E:\`}
	_, err := newTestMounter(sys).Mount(context.Background(), writeImage(t, "usb.img", hardDiskImage()))
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("Mount() = %v, očekávána ErrUnsupportedFormat", err)
	}
	if len(sys.calls) != 0 {
		t.Fatalf("nepodporovaný obraz se neměl otevírat: %v", sys.calls)
	}
}

func TestMountReportsMissingFile(t *testing.T) {
	sys := &fakeSystem{volume: `E:\`}
	missing := filepath.Join(t.TempDir(), "chybí.iso")
	_, err := newTestMounter(sys).Mount(context.Background(), missing)
	if !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("Mount() = %v, očekávána ErrImageNotFound", err)
	}
}

func TestMountReportsPermissionFailure(t *testing.T) {
	sys := &fakeSystem{attachErr: fmt.Errorf("%w: test", ErrAccessDenied), volume: `E:\`}
	_, err := newTestMounter(sys).Mount(context.Background(), writeImage(t, "app.iso", isoImage()))
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("Mount() = %v, očekávána ErrAccessDenied", err)
	}
	if sys.called("detach") || !sys.called("close") {
		t.Fatalf("po neúspěšném připojení se má jen zavřít popisovač: %v", sys.calls)
	}
}

func TestMountReportsMissingVolumeAndDetaches(t *testing.T) {
	sys := &fakeSystem{}
	_, err := newTestMounter(sys).Mount(context.Background(), writeImage(t, "app.iso", isoImage()))
	if !errors.Is(err, ErrNoVolume) {
		t.Fatalf("Mount() = %v, očekávána ErrNoVolume", err)
	}
	if sys.attached || !sys.called("close") {
		t.Fatalf("obraz bez svazku zůstal připojený: %v", sys.calls)
	}
}

func TestMountHonoursCancellation(t *testing.T) {
	path := writeImage(t, "app.iso", isoImage())

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	sys := &fakeSystem{volume: `E:\`}
	if _, err := newTestMounter(sys).Mount(canceled, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Mount() = %v, očekáváno context.Canceled", err)
	}
	if len(sys.calls) != 0 {
		t.Fatalf("zrušené připojení nemělo nic volat: %v", sys.calls)
	}

	// Zrušení během čekání na svazek musí vlastní obraz odpojit.
	sys = &fakeSystem{}
	mounter := newTestMounter(sys)
	mounter.volumeWait = time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := mounter.Mount(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Mount() = %v, očekáváno context.DeadlineExceeded", err)
	}
	if sys.attached {
		t.Fatal("obraz zůstal po zrušení připojený")
	}
}
