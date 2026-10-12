package main

import (
	"context"
	"fmt"
	"runtime"

	"jandy.cz/ess-provisioner/internal/admin"
	"jandy.cz/ess-provisioner/internal/config"
	"jandy.cz/ess-provisioner/internal/ui"
)

func main() {
	cfg, err := config.LoadEmbedded()
	if err != nil {
		admin.ShowError("Neplatná vložená konfigurace", err.Error())
		return
	}
	if !admin.IsElevated() {
		if err := admin.RelaunchElevated(); err != nil {
			admin.ShowError(cfg.ProductName, fmt.Sprintf("Program vyžaduje oprávnění správce.\n\n%v", err))
		}
		return
	}

	apps, appListErr := config.FetchRemoteApps(context.Background(), cfg)
	if appListErr != nil {
		cfg.Apps = nil
		admin.ShowError(
			cfg.ProductName,
			fmt.Sprintf(
				"Seznam aplikací se nepodařilo načíst ze vzdálené konfigurace.\n\n%v\n\nInstalace aplikací bude zakázána a seznam zůstane prázdný. Ostatní funkce programu zůstanou dostupné.",
				appListErr,
			),
		)
	} else {
		cfg.Apps = apps
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ui.Run(cfg, appListErr); err != nil {
		admin.ShowError(cfg.ProductName, err.Error())
	}
}
