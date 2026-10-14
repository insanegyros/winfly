package install

import (
	"context"
	"fmt"
	"slices"

	"jandy.cz/winfly/internal/config"
	"jandy.cz/winfly/internal/diskimage"
)

// mountDiskImage připojí obraz disku a vrátí kořen jeho svazku a funkci pro
// odpojení. Testy ji nahrazují, aby nepotřebovaly skutečné připojení.
var mountDiskImage = func(ctx context.Context, imagePath string, log Logger) (root string, unmount func() error, err error) {
	img, err := diskimage.NewMounter().Mount(ctx, imagePath)
	if err != nil {
		return "", nil, err
	}
	if !img.Owned {
		log("Obraz disku už byl připojen jinou aplikací; po instalaci zůstane připojený")
	}
	return img.MountPath, img.Unmount, nil
}

// runFromImage připojí obraz, ověří instalátor podle autorun.inf, spustí ho
// stejným způsobem jako samostatný instalátor EXE nebo MSI a obraz odpojí až
// po jeho ukončení.
func runFromImage(ctx context.Context, app config.AppConfig, imagePath string, log Logger) (Result, error) {
	log("Připojuje se obraz disku aplikace %s", app.Name)
	root, unmount, err := mountDiskImage(ctx, imagePath, log)
	if err != nil {
		return Result{}, fmt.Errorf("připojení obrazu disku aplikace %s: %w", app.Name, err)
	}
	log("Obraz disku je připojen jako %s", root)
	defer func() {
		if unmountErr := unmount(); unmountErr != nil {
			log("Upozornění: obraz disku aplikace %s se nepodařilo odpojit: %v", app.Name, unmountErr)
			return
		}
		log("Obraz disku aplikace %s byl odpojen", app.Name)
	}()

	installer, err := diskimage.ReadAutorun(root)
	if err != nil {
		return Result{}, fmt.Errorf("ověření instalátoru v obrazu disku aplikace %s: %w", app.Name, err)
	}
	l, err := imageLaunch(app, installer, root)
	if err != nil {
		return Result{}, err
	}

	log("Spouští se instalátor aplikace %s z obrazu disku: %s (položka %s v autorun.inf)", app.Name, installer.RelativePath, installer.Entry)
	return runCommand(ctx, app, l, log)
}

// imageLaunch sestaví spuštění instalátoru z obrazu. Parametry z autorun.inf
// jsou první, za nimi následují parametry z konfigurace aplikace; žádné jiné
// (např. pro tichou instalaci) se nepřidávají. Pracovním adresářem je kořen
// obrazu, stejně jako při automatickém přehrání ve Windows, protože k němu
// se vztahují relativní cesty v autorun.inf.
func imageLaunch(app config.AppConfig, installer diskimage.AutorunInstaller, root string) (launch, error) {
	inner := app
	inner.InstallerType = installer.Type
	inner.Arguments = slices.Concat(installer.Arguments, app.Arguments)

	program, args, err := commandFor(inner, installer.Path)
	if err != nil {
		return launch{}, err
	}
	return launch{
		program: program,
		args:    args,
		dir:     root,
	}, nil
}
