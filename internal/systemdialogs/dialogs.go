package systemdialogs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// OpenPowerOptions otevře klasickou položku Možnosti napájení v Ovládacích panelech.
func OpenPowerOptions() error {
	return startDetached(systemExecutable("control.exe"), "/name", "Microsoft.PowerOptions", "/page", "pageGlobalSettings")
}

// OpenDomainOrWorkgroup otevře stránku Název počítače ve Vlastnostech systému,
// kde Windows umožňuje spravovat název, doménu a pracovní skupinu počítače.
func OpenDomainOrWorkgroup() error {
	return startDetached(systemExecutable("SystemPropertiesComputerName.exe"))
}

func systemExecutable(name string) string {
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		return filepath.Join(systemRoot, "System32", name)
	}
	return name
}

func startDetached(program string, args ...string) error {
	cmd := exec.Command(program, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spuštění programu %s: %w", filepath.Base(program), err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("uvolnění popisovače procesu %s: %w", filepath.Base(program), err)
	}
	return nil
}
