package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"jandy.cz/winfly/internal/config"
	"jandy.cz/winfly/internal/diskimage"
)

// Proměnné prostředí, kterými testy řídí pomocný instalátor (viz TestMain).
const (
	helperOutputEnv   = "WINFLY_TEST_INSTALLER_OUTPUT"
	helperExitCodeEnv = "WINFLY_TEST_INSTALLER_EXIT_CODE"
)

// helperRun je záznam o spuštění pomocného instalátoru.
type helperRun struct {
	Args []string
	Dir  string
}

// TestMain umožňuje použít testovací binárku jako instalátor: pokud je
// nastavena proměnná helperOutputEnv, zapíše své parametry a pracovní
// adresář a skončí s kódem z helperExitCodeEnv.
func TestMain(m *testing.M) {
	output := os.Getenv(helperOutputEnv)
	if output == "" {
		os.Exit(m.Run())
	}
	dir, _ := os.Getwd()
	data, _ := json.Marshal(helperRun{Args: os.Args[1:], Dir: dir})
	_ = os.WriteFile(output, data, 0o600)
	code, _ := strconv.Atoi(os.Getenv(helperExitCodeEnv))
	os.Exit(code)
}

func TestImageLaunch(t *testing.T) {
	root := `E:\`
	tests := []struct {
		name        string
		installer   diskimage.AutorunInstaller
		appArgs     []string
		wantProgram string
		wantArgs    []string
	}{
		{
			name:        "EXE bez parametrů",
			installer:   diskimage.AutorunInstaller{Path: `E:\setup.exe`, Type: "exe", Arguments: []string{}},
			wantProgram: `E:\setup.exe`,
			wantArgs:    []string{},
		},
		{
			name:        "EXE s parametry z autorun.inf i konfigurace",
			installer:   diskimage.AutorunInstaller{Path: `E:\x64\setup.exe`, Type: "exe", Arguments: []string{"/config", "Moje nastavení.xml"}},
			appArgs:     []string{"/log", `C:\Logy\instalace office.txt`},
			wantProgram: `E:\x64\setup.exe`,
			wantArgs:    []string{"/config", "Moje nastavení.xml", "/log", `C:\Logy\instalace office.txt`},
		},
		{
			name:        "MSI přes msiexec",
			installer:   diskimage.AutorunInstaller{Path: `E:\Install.msi`, Type: "msi", Arguments: []string{"/qn"}},
			appArgs:     []string{"REBOOT=ReallySuppress"},
			wantProgram: "msiexec.exe",
			wantArgs:    []string{"/i", `E:\Install.msi`, "/qn", "REBOOT=ReallySuppress"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := config.AppConfig{InstallerType: "iso", Arguments: tt.appArgs}
			l, err := imageLaunch(app, tt.installer, root)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(l.program) != filepath.Base(tt.wantProgram) {
				t.Errorf("program = %q, očekáváno %q", l.program, tt.wantProgram)
			}
			if fmt.Sprintf("%q", l.args) != fmt.Sprintf("%q", tt.wantArgs) {
				t.Errorf("parametry = %q, očekáváno %q", l.args, tt.wantArgs)
			}
			if l.dir != root {
				t.Errorf("pracovní adresář = %q, očekáván kořen obrazu %q", l.dir, root)
			}
		})
	}
}

// fakeImage připraví adresář, který test použije jako kořen připojeného
// obrazu, a nahradí připojování obrazu.
type fakeImage struct {
	root             string
	unmounted        bool
	ranBeforeUnmount bool
	output           string
}

func newFakeImage(t *testing.T, autorun string) *fakeImage {
	t.Helper()
	img := &fakeImage{
		root:   filepath.Join(t.TempDir(), "Připojený obraz"),
		output: filepath.Join(t.TempDir(), "spuštění.json"),
	}
	if err := os.MkdirAll(filepath.Join(img.root, "setup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if autorun != "" {
		writeFile(t, filepath.Join(img.root, "autorun.inf"), []byte(autorun))
	}
	copyExecutable(t, filepath.Join(img.root, "setup", "Setup.exe"))

	previous := mountDiskImage
	mountDiskImage = func(context.Context, string, Logger) (string, func() error, error) {
		return img.root, img.unmount, nil
	}
	t.Cleanup(func() { mountDiskImage = previous })
	t.Setenv(helperOutputEnv, img.output)
	return img
}

func (img *fakeImage) unmount() error {
	_, err := os.Stat(img.output)
	img.ranBeforeUnmount = err == nil
	img.unmounted = true
	return nil
}

func (img *fakeImage) run(t *testing.T) helperRun {
	t.Helper()
	data, err := os.ReadFile(img.output)
	if err != nil {
		t.Fatalf("instalátor se nespustil: %v", err)
	}
	var run helperRun
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestRunFromImage(t *testing.T) {
	img := newFakeImage(t, "[AutoRun]\r\nopen=\"setup\\Setup.exe\" \"/dir=C:\\Program Files\\Aplikace\"\r\n")
	t.Setenv(helperExitCodeEnv, "3010")

	app := config.AppConfig{
		Name:             "Office",
		InstallerType:    "auto",
		Arguments:        []string{"/log", "instalace office.txt"},
		SuccessExitCodes: []int{0, 3010},
	}
	result, err := Run(context.Background(), app, writeImageFile(t), nil)
	if err != nil {
		t.Fatalf("Run() vrátil chybu: %v", err)
	}
	if result.ExitCode != 3010 || !result.RestartRequired {
		t.Fatalf("výsledek = %+v, očekáván kód 3010 s restartem", result)
	}

	run := img.run(t)
	wantArgs := []string{`/dir=C:\Program Files\Aplikace`, "/log", "instalace office.txt"}
	if fmt.Sprintf("%q", run.Args) != fmt.Sprintf("%q", wantArgs) {
		t.Errorf("parametry = %q, očekáváno %q", run.Args, wantArgs)
	}
	if !sameDir(run.Dir, img.root) {
		t.Errorf("pracovní adresář = %q, očekáván kořen obrazu %q", run.Dir, img.root)
	}
	if !img.unmounted || !img.ranBeforeUnmount {
		t.Errorf("obraz se má odpojit až po skončení instalátoru (odpojen=%t, po spuštění=%t)", img.unmounted, img.ranBeforeUnmount)
	}
}

func TestRunFromImageRejectsFailedInstaller(t *testing.T) {
	img := newFakeImage(t, "[AutoRun]\nopen=setup\\setup.exe\n")
	t.Setenv(helperExitCodeEnv, "5")

	app := config.AppConfig{Name: "Office", InstallerType: "iso"}
	result, err := Run(context.Background(), app, writeImageFile(t), nil)
	if err == nil || result.ExitCode != 5 {
		t.Fatalf("Run() = %+v, %v; očekávána chyba s kódem 5", result, err)
	}
	if !img.unmounted {
		t.Fatal("obraz zůstal po neúspěšné instalaci připojený")
	}
}

func TestRunFromImageWithoutAutorun(t *testing.T) {
	img := newFakeImage(t, "")

	app := config.AppConfig{Name: "Office", InstallerType: "img"}
	_, err := Run(context.Background(), app, writeImageFile(t), nil)
	if !errors.Is(err, diskimage.ErrNoAutorun) {
		t.Fatalf("Run() = %v, očekávána ErrNoAutorun", err)
	}
	var imageErr *ImageError
	if !errors.As(err, &imageErr) || imageErr.Stage != StageAutorun {
		t.Fatalf("Run() = %v, očekávána ImageError v kroku %q", err, StageAutorun)
	}
	if _, statErr := os.Stat(img.output); statErr == nil {
		t.Fatal("bez autorun.inf se nesmí nic spustit")
	}
	if !img.unmounted {
		t.Fatal("obraz zůstal připojený")
	}
}

func TestRunFromImageStopsWhenMountFails(t *testing.T) {
	img := newFakeImage(t, "[AutoRun]\nopen=setup\\setup.exe\n")
	mountDiskImage = func(context.Context, string, Logger) (string, func() error, error) {
		return "", nil, fmt.Errorf("%w: test", diskimage.ErrUnsupportedFormat)
	}

	app := config.AppConfig{Name: "Office", InstallerType: "iso"}
	_, err := Run(context.Background(), app, writeImageFile(t), nil)
	var imageErr *ImageError
	if !errors.As(err, &imageErr) || imageErr.Stage != StageMount || !errors.Is(err, diskimage.ErrUnsupportedFormat) {
		t.Fatalf("Run() = %v, očekávána ImageError v kroku %q", err, StageMount)
	}
	if _, statErr := os.Stat(img.output); statErr == nil {
		t.Fatal("nepřipojený obraz nesmí nic spustit")
	}
}

func TestRunFromImageWaitsForOtherInstallOfSameImage(t *testing.T) {
	img := newFakeImage(t, "[AutoRun]\nopen=setup\\setup.exe\n")
	imagePath := writeImageFile(t)
	unlock, err := lockImage(context.Background(), imagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	app := config.AppConfig{Name: "Office", InstallerType: "iso"}
	_, err = Run(ctx, app, imagePath, nil)
	var imageErr *ImageError
	if !errors.As(err, &imageErr) || imageErr.Stage != StageLock {
		t.Fatalf("Run() = %v, očekávána ImageError v kroku %q", err, StageLock)
	}
	if img.unmounted {
		t.Fatal("obraz používaný jinou instalací se nesmí připojit ani odpojit")
	}
}

func TestRunFromImageRequiresWait(t *testing.T) {
	img := newFakeImage(t, "[AutoRun]\nopen=setup\\setup.exe\n")

	wait := false
	app := config.AppConfig{Name: "Office", InstallerType: "iso", Wait: &wait}
	if _, err := Run(context.Background(), app, writeImageFile(t), nil); err == nil {
		t.Fatal("Run() neočekávaně přijal wait=false pro obraz disku")
	}
	if img.unmounted {
		t.Fatal("obraz se neměl vůbec připojit")
	}
}

func writeImageFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Office 2024.iso")
	writeFile(t, path, []byte("obsah obrazu"))
	return path
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// copyExecutable zkopíruje testovací binárku, která poslouží jako instalátor.
func copyExecutable(t *testing.T, target string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	destination, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if _, err := io.Copy(destination, source); err != nil {
		t.Fatal(err)
	}
}

func sameDir(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
