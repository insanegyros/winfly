package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"jandy.cz/winfly/internal/config"
)

type Logger func(format string, args ...any)

type Result struct {
	ExitCode        int
	StartedOnly     bool
	RestartRequired bool
}

func Run(ctx context.Context, app config.AppConfig, installerPath string, log Logger) (Result, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	kind, err := installerKind(app, installerPath)
	if err != nil {
		return Result{}, err
	}
	if kind == "msi" && !app.ShouldWait() {
		return Result{}, errors.New("instalátory MSI vyžadují wait=true, aby balíček zůstal uzamčený až do ukončení služby Windows Installer")
	}

	info, err := os.Stat(installerPath)
	if err != nil {
		return Result{}, fmt.Errorf("zjištění informací o instalátoru: %w", err)
	}
	if info.IsDir() {
		return Result{}, errors.New("cesta k instalátoru odkazuje na adresář")
	}

	verifiedFile, err := lockAndVerifyInstaller(installerPath, app.SHA256)
	if err != nil {
		return Result{}, fmt.Errorf("uzamčení instalátoru: %w", err)
	}
	defer verifiedFile.Close()
	if !app.HasValidSHA256() {
		log("Upozornění: aplikace %s se spouští bez ověření SHA-256", app.Name)
	}

	program, args, err := commandFor(app, installerPath)
	if err != nil {
		return Result{}, err
	}
	log("Spouští se instalátor aplikace %s", app.Name)

	var cmd *exec.Cmd
	if app.ShouldWait() {
		cmd = exec.CommandContext(ctx, program, args...)
	} else {
		cmd = exec.Command(program, args...)
	}
	cmd.Dir = filepath.Dir(installerPath)

	if !app.ShouldWait() {
		if err := cmd.Start(); err != nil {
			return Result{}, fmt.Errorf("spuštění aplikace %s: %w", app.Name, err)
		}
		if err := cmd.Process.Release(); err != nil {
			return Result{}, fmt.Errorf("uvolnění popisovače procesu aplikace %s: %w", app.Name, err)
		}
		return Result{StartedOnly: true}, nil
	}

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return Result{}, fmt.Errorf("spuštění aplikace %s: %w", app.Name, err)
		}
		exitCode = exitErr.ExitCode()
	}
	if !containsExitCode(app.SuccessExitCodes, exitCode) {
		return Result{ExitCode: exitCode}, fmt.Errorf("aplikace %s skončila s kódem %d", app.Name, exitCode)
	}

	result := Result{
		ExitCode:        exitCode,
		RestartRequired: exitCode == 1641 || exitCode == 3010,
	}
	if result.RestartRequired {
		log("aplikace %s byla dokončena a oznámila požadavek na restart", app.Name)
	} else {
		log("aplikace %s byla úspěšně dokončena", app.Name)
	}
	return result, nil
}

func commandFor(app config.AppConfig, installerPath string) (string, []string, error) {
	kind, err := installerKind(app, installerPath)
	if err != nil {
		return "", nil, err
	}

	switch kind {
	case "exe":
		return installerPath, append([]string(nil), app.Arguments...), nil
	case "msi":
		msiexec := "msiexec.exe"
		if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
			msiexec = filepath.Join(systemRoot, "System32", "msiexec.exe")
		}
		args := []string{"/i", installerPath}
		args = append(args, app.Arguments...)
		return msiexec, args, nil
	default:
		return "", nil, fmt.Errorf("nepodporovaný typ instalátoru %q", kind)
	}
}

func installerKind(app config.AppConfig, installerPath string) (string, error) {
	kind := strings.ToLower(strings.TrimSpace(app.InstallerType))
	if kind == "" || kind == "auto" {
		switch strings.ToLower(filepath.Ext(installerPath)) {
		case ".msi":
			kind = "msi"
		case ".exe":
			kind = "exe"
		default:
			return "", fmt.Errorf("typ instalátoru nelze určit z názvu %q", installerPath)
		}
	}
	if kind != "exe" && kind != "msi" {
		return "", fmt.Errorf("nepodporovaný typ instalátoru %q", kind)
	}
	return kind, nil
}

func containsExitCode(codes []int, value int) bool {
	if len(codes) == 0 {
		return value == 0 || value == 1641 || value == 3010
	}
	for _, code := range codes {
		if code == value {
			return true
		}
	}
	return false
}
