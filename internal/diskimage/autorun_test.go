package diskimage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const testRoot = `E:\`

// imageFS vrátí obsah připojeného obrazu s autorun.inf a zadanými soubory.
func imageFS(autorun string, files ...string) fstest.MapFS {
	fsys := fstest.MapFS{}
	if autorun != "" {
		fsys["AUTORUN.INF"] = &fstest.MapFile{Data: []byte(autorun)}
	}
	for _, name := range files {
		fsys[name] = &fstest.MapFile{Data: []byte("MZ")}
	}
	return fsys
}

func TestReadAutorunValid(t *testing.T) {
	tests := []struct {
		name      string
		fsys      fstest.MapFS
		wantPath  string
		wantArgs  []string
		wantType  string
		wantEntry string
	}{
		{
			name:      "open",
			fsys:      imageFS("[AutoRun]\r\nopen=setup.exe\r\nicon=setup.ico\r\n", "setup.exe"),
			wantPath:  "setup.exe",
			wantArgs:  []string{},
			wantType:  "exe",
			wantEntry: "open",
		},
		{
			name:      "shellexecute s parametry",
			fsys:      imageFS("[autorun]\nshellexecute=Install.msi /qn REBOOT=ReallySuppress\n", "Install.msi"),
			wantPath:  "Install.msi",
			wantArgs:  []string{"/qn", "REBOOT=ReallySuppress"},
			wantType:  "msi",
			wantEntry: "shellexecute",
		},
		{
			name:      "uvozovky, mezery a podadresář",
			fsys:      imageFS("[AUTORUN]\nOPEN = \"x64\\Instalace Office.exe\" /config \"Moje nastavení.xml\"\n", "x64/Instalace Office.exe"),
			wantPath:  "x64/Instalace Office.exe",
			wantArgs:  []string{"/config", "Moje nastavení.xml"},
			wantType:  "exe",
			wantEntry: "open",
		},
		{
			name:      "jiná velikost písmen a úvodní lomítko",
			fsys:      imageFS("[AutoRun]\nopen=\\setup\\setup.exe\n", "SETUP/SETUP.EXE"),
			wantPath:  "SETUP/SETUP.EXE",
			wantArgs:  []string{},
			wantType:  "exe",
			wantEntry: "open",
		},
		{
			name:      "komentáře, jiné oddíly a stejné open i shellexecute",
			fsys:      imageFS("; komentář\n[Content]\nopen=jiny.exe\n\n[AutoRun]\nlabel=Office\nopen=setup.exe\nshellexecute=setup.exe\n", "setup.exe"),
			wantPath:  "setup.exe",
			wantArgs:  []string{},
			wantType:  "exe",
			wantEntry: "open",
		},
		{
			name:      "UTF-8 s BOM",
			fsys:      imageFS("\uFEFF[AutoRun]\nopen=instalátor.exe\n", "instalátor.exe"),
			wantPath:  "instalátor.exe",
			wantArgs:  []string{},
			wantType:  "exe",
			wantEntry: "open",
		},
		{
			name:      "UTF-16 LE s BOM",
			fsys:      imageFS(utf16LE("[AutoRun]\r\nopen=setup.exe /s\r\n"), "setup.exe"),
			wantPath:  "setup.exe",
			wantArgs:  []string{"/s"},
			wantType:  "exe",
			wantEntry: "open",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readAutorun(tt.fsys, testRoot)
			if err != nil {
				t.Fatalf("readAutorun() vrátil chybu: %v", err)
			}
			wantFullPath := filepath.Join(testRoot, filepath.FromSlash(tt.wantPath))
			if got.RelativePath != tt.wantPath || got.Path != wantFullPath {
				t.Errorf("cesta = %q (%q), očekáváno %q (%q)", got.RelativePath, got.Path, tt.wantPath, wantFullPath)
			}
			if fmt.Sprintf("%q", got.Arguments) != fmt.Sprintf("%q", tt.wantArgs) {
				t.Errorf("parametry = %q, očekáváno %q", got.Arguments, tt.wantArgs)
			}
			if got.Type != tt.wantType || got.Entry != tt.wantEntry {
				t.Errorf("typ a položka = %q %q, očekáváno %q %q", got.Type, got.Entry, tt.wantType, tt.wantEntry)
			}
		})
	}
}

func TestReadAutorunRejects(t *testing.T) {
	tests := []struct {
		name        string
		fsys        fstest.MapFS
		wantErr     error
		wantInError string
	}{
		// Chybějící autorun.inf; setup.exe se nikdy nehledá náhradou.
		{name: "chybí autorun.inf", fsys: imageFS("", "setup.exe"), wantErr: ErrNoAutorun},
		{name: "autorun.inf je adresář", fsys: fstest.MapFS{"autorun.inf/x": {}}, wantErr: ErrInvalidAutorun},

		// Chybějící cíl.
		{name: "instalátor neexistuje", fsys: imageFS("[AutoRun]\nopen=setup.exe\n"), wantErr: ErrInstallerNotFound},
		{name: "adresář neexistuje", fsys: imageFS("[AutoRun]\nopen=x64\\setup.exe\n", "setup.exe"), wantErr: ErrInstallerNotFound},
		{name: "cíl je adresář", fsys: imageFS("[AutoRun]\nopen=setup.exe\n", "setup.exe/readme.txt"), wantErr: ErrInstallerNotFound},
		{name: "bez uvozovek se mezera bere jako konec názvu", fsys: imageFS("[AutoRun]\nopen=My Setup.exe\n", "My Setup.exe"), wantErr: ErrInstallerNotFound},

		// Chybná konfigurace.
		{name: "chybí oddíl AutoRun", fsys: imageFS("[Content]\nopen=setup.exe\n", "setup.exe"), wantErr: ErrInvalidAutorun, wantInError: "[AutoRun]"},
		{name: "neukončený oddíl", fsys: imageFS("[AutoRun\nopen=setup.exe\n", "setup.exe"), wantErr: ErrInvalidAutorun, wantInError: "řádek 1"},
		{name: "řádek bez rovnítka", fsys: imageFS("[AutoRun]\nopen setup.exe\n", "setup.exe"), wantErr: ErrInvalidAutorun, wantInError: "řádek 2"},
		{name: "prázdné open", fsys: imageFS("[AutoRun]\nopen=\n", "setup.exe"), wantErr: ErrInvalidAutorun, wantInError: "prázdná"},
		{name: "neukončené uvozovky", fsys: imageFS("[AutoRun]\nopen=\"setup.exe /q\n", "setup.exe"), wantErr: ErrInvalidAutorun, wantInError: "uvozovky"},
		{name: "různé open a shellexecute", fsys: imageFS("[AutoRun]\nopen=a.exe\nshellexecute=b.exe\n", "a.exe", "b.exe"), wantErr: ErrInvalidAutorun, wantInError: "různé"},
		{name: "nepodporovaný typ programu", fsys: imageFS("[AutoRun]\nopen=install.bat\n", "install.bat"), wantErr: ErrInvalidAutorun, wantInError: ".exe"},
		{name: "neplatné kódování", fsys: imageFS("[AutoRun]\nopen=instal\xe1tor.exe\n", "instalátor.exe"), wantErr: ErrInvalidAutorun, wantInError: "kódování"},
		{name: "příliš velký soubor", fsys: imageFS("[AutoRun]\n"+strings.Repeat(";", maxAutorunBytes)+"\n", "setup.exe"), wantErr: ErrInvalidAutorun, wantInError: "větší"},

		// Jen ikona nebo popisek.
		{name: "jen ikona", fsys: imageFS("[AutoRun]\nicon=setup.exe,0\nlabel=Office\n", "setup.exe"), wantErr: ErrNoInstallerEntry},
		{name: "prázdný oddíl", fsys: imageFS("[AutoRun]\n", "setup.exe"), wantErr: ErrNoInstallerEntry},

		// Nebezpečné cesty mimo obraz.
		{name: "nadřazený adresář", fsys: imageFS("[AutoRun]\nopen=..\\setup.exe\n", "setup.exe"), wantErr: ErrUnsafeInstallerPath},
		{name: "nadřazený adresář uprostřed", fsys: imageFS("[AutoRun]\nopen=x64\\..\\..\\setup.exe\n", "setup.exe"), wantErr: ErrUnsafeInstallerPath},
		{name: "písmeno jednotky", fsys: imageFS("[AutoRun]\nopen=C:\\Windows\\System32\\cmd.exe\n"), wantErr: ErrUnsafeInstallerPath},
		{name: "relativní k jednotce", fsys: imageFS("[AutoRun]\nopen=C:setup.exe\n", "setup.exe"), wantErr: ErrUnsafeInstallerPath},
		{name: "cesta UNC", fsys: imageFS("[AutoRun]\nopen=\\\\server\\share\\setup.exe\n"), wantErr: ErrUnsafeInstallerPath},
		{name: "proměnná prostředí", fsys: imageFS("[AutoRun]\nopen=%windir%\\system32\\cmd.exe\n"), wantErr: ErrUnsafeInstallerPath},
		{name: "datový proud", fsys: imageFS("[AutoRun]\nopen=setup.exe:skryty.exe\n", "setup.exe"), wantErr: ErrUnsafeInstallerPath},
		{name: "rezervovaný název", fsys: imageFS("[AutoRun]\nopen=CON\\setup.exe\n"), wantErr: ErrUnsafeInstallerPath},
		{name: "zástupné znaky", fsys: imageFS("[AutoRun]\nopen=set*.exe\n", "setup.exe"), wantErr: ErrUnsafeInstallerPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readAutorun(tt.fsys, testRoot)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("readAutorun() = %v, očekáváno %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantInError) {
				t.Fatalf("chyba %q neobsahuje %q", err, tt.wantInError)
			}
		})
	}
}

func utf16LE(text string) string {
	var b strings.Builder
	b.WriteString("\xFF\xFE")
	for _, r := range text {
		b.WriteByte(byte(r))
		b.WriteByte(byte(r >> 8))
	}
	return b.String()
}

// ReadAutorun čte skutečný adresář, jako by šlo o kořen připojeného svazku.
func TestReadAutorunFromDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Připojený obraz")
	installer := filepath.Join(root, "Instalace", "setup.exe")
	if err := os.MkdirAll(filepath.Dir(installer), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installer, []byte("MZ"), 0o600); err != nil {
		t.Fatal(err)
	}
	autorun := "[AutoRun]\r\nopen=\"Instalace\\setup.exe\" /q\r\n"
	if err := os.WriteFile(filepath.Join(root, "Autorun.inf"), []byte(autorun), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadAutorun(root)
	if err != nil {
		t.Fatalf("ReadAutorun() vrátil chybu: %v", err)
	}
	if got.Path != installer {
		t.Fatalf("Path = %q, očekáváno %q", got.Path, installer)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Fatalf("vrácená cesta neexistuje: %v", err)
	}
}
