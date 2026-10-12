package download

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// secureCacheDirectory odstraní zděděná oprávnění a udělí úplné řízení pouze
// účtům LocalSystem a místní skupině Administrators. Běžný uživatel tak nemůže
// vyměnit ověřený instalátor mezi kontrolou a spuštěním.
func secureCacheDirectory(path string) error {
	program := "icacls.exe"
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		program = filepath.Join(systemRoot, "System32", "icacls.exe")
	}
	cmd := exec.Command(
		program,
		path,
		"/inheritance:r",
		"/grant:r",
		"*S-1-5-18:(OI)(CI)F",
		"*S-1-5-32-544:(OI)(CI)F",
		"/T",
		"/C",
		"/Q",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, message)
	}
	return nil
}
