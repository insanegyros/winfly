package install

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jandy.cz/winfly/internal/config"
)

func TestCommandForMSI(t *testing.T) {
	program, args, err := commandFor(config.AppConfig{InstallerType: "auto", Arguments: []string{"/qn"}}, filepath.Join("tmp", "app.msi"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(program) != "msiexec.exe" {
		t.Fatalf("program = %q", program)
	}
	if len(args) != 3 || args[0] != "/i" || args[2] != "/qn" {
		t.Fatalf("parametry = %#v", args)
	}
}

func TestContainsExitCode(t *testing.T) {
	if !containsExitCode([]int{0, 3010}, 3010) {
		t.Fatal("kód 3010 měl být přijat")
	}
	if containsExitCode([]int{0}, 5) {
		t.Fatal("kód 5 byl neočekávaně přijat")
	}
}

func TestLockAndVerifyInstaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installer.exe")
	payload := []byte("testovaci obsah instalatoru")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	locked, err := lockAndVerifyInstaller(path, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := lockAndVerifyInstaller(path, strings.Repeat("a", 64)); err == nil {
		t.Fatal("lockAndVerifyInstaller neočekávaně přijal nesouhlasící otisk")
	}
}

func TestLockAllowsMissingOrInvalidSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installer.exe")
	if err := os.WriteFile(path, []byte("testovaci obsah instalatoru"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sha := range []string{"", "neplatny"} {
		locked, err := lockAndVerifyInstaller(path, sha)
		if err != nil {
			t.Fatalf("SHA-256 %q: %v", sha, err)
		}
		if err := locked.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
