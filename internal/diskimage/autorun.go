package diskimage

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"jandy.cz/winfly/internal/config"
)

const (
	autorunFileName = "autorun.inf"
	// Skutečné soubory autorun.inf mají několik set bajtů.
	maxAutorunBytes = 64 * 1024
)

var (
	// ErrNoAutorun znamená, že kořen obrazu neobsahuje autorun.inf.
	ErrNoAutorun = errors.New("obraz disku neobsahuje soubor autorun.inf, a proto nejde o platný instalátor aplikace")
	// ErrInvalidAutorun znamená, že autorun.inf nemá platný obsah.
	ErrInvalidAutorun = errors.New("soubor autorun.inf je neplatný")
	// ErrNoInstallerEntry znamená, že autorun.inf neurčuje program k
	// instalaci (např. obsahuje jen ikonu nebo popisek).
	ErrNoInstallerEntry = errors.New("soubor autorun.inf neurčuje instalátor (chybí položka open nebo shellexecute)")
	// ErrUnsafeInstallerPath znamená, že cesta k instalátoru míří mimo obraz
	// nebo není bezpečná.
	ErrUnsafeInstallerPath = errors.New("cesta k instalátoru v autorun.inf není bezpečná")
	// ErrInstallerNotFound znamená, že instalátor uvedený v autorun.inf
	// v obrazu neexistuje.
	ErrInstallerNotFound = errors.New("instalátor uvedený v autorun.inf v obrazu disku neexistuje")
)

// AutorunInstaller je ověřený instalátor z autorun.inf. Nic se nespouští.
type AutorunInstaller struct {
	// Path je úplná cesta k instalátoru na připojeném svazku.
	Path string
	// RelativePath je cesta v rámci obrazu s lomítky, např. `x64/setup.exe`,
	// se skutečnou velikostí písmen ze svazku.
	RelativePath string
	// Arguments jsou parametry z autorun.inf.
	Arguments []string
	// Type je config.InstallerTypeEXE nebo config.InstallerTypeMSI.
	Type string
	// Entry je položka, ze které instalátor pochází: open nebo shellexecute.
	Entry string
}

// ReadAutorun najde autorun.inf v kořeni připojeného svazku root, například
// `E:\`, a ověří instalátor, na který odkazuje. Instalátor se nikdy nehledá
// jinak než přes autorun.inf.
func ReadAutorun(root string) (AutorunInstaller, error) {
	return readAutorun(os.DirFS(root), root)
}

func readAutorun(fsys fs.FS, root string) (AutorunInstaller, error) {
	content, err := readAutorunFile(fsys)
	if err != nil {
		return AutorunInstaller{}, err
	}
	entry, command, err := installerEntry(content)
	if err != nil {
		return AutorunInstaller{}, err
	}

	program, args, err := splitCommandLine(command)
	if err != nil {
		return AutorunInstaller{}, fmt.Errorf("%w: položka %s: %w", ErrInvalidAutorun, entry, err)
	}
	relative, err := resolveInstallerPath(fsys, program)
	if err != nil {
		return AutorunInstaller{}, err
	}
	kind, err := config.ResolveInstallerType(config.InstallerTypeAuto, relative)
	if err != nil || config.IsDiskImageType(kind) {
		return AutorunInstaller{}, fmt.Errorf("%w: %q není program .exe ani balíček .msi", ErrInvalidAutorun, program)
	}

	return AutorunInstaller{
		Path:         filepath.Join(root, filepath.FromSlash(relative)),
		RelativePath: relative,
		Arguments:    args,
		Type:         kind,
		Entry:        entry,
	}, nil
}

// readAutorunFile najde autorun.inf v kořeni bez ohledu na velikost písmen
// a vrátí jeho text.
func readAutorunFile(fsys fs.FS) (string, error) {
	name, err := findEntry(fsys, ".", autorunFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoAutorun
	}
	if err != nil {
		return "", fmt.Errorf("hledání souboru autorun.inf: %w", err)
	}

	f, err := fsys.Open(name)
	if err != nil {
		return "", fmt.Errorf("otevření souboru autorun.inf: %w", err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: autorun.inf není běžný soubor", ErrInvalidAutorun)
	}

	data, err := io.ReadAll(io.LimitReader(f, maxAutorunBytes+1))
	if err != nil {
		return "", fmt.Errorf("čtení souboru autorun.inf: %w", err)
	}
	if len(data) > maxAutorunBytes {
		return "", fmt.Errorf("%w: soubor je větší než %d bajtů", ErrInvalidAutorun, maxAutorunBytes)
	}
	return decodeAutorun(data)
}

// decodeAutorun převede obsah na text. Podporuje UTF-8 (i s BOM) a UTF-16 LE
// s BOM, které vytváří Poznámkový blok.
func decodeAutorun(data []byte) (string, error) {
	if utf16LE := []byte{0xFF, 0xFE}; bytes.HasPrefix(data, utf16LE) {
		data = data[len(utf16LE):]
		if len(data)%2 != 0 {
			return "", fmt.Errorf("%w: poškozený text UTF-16", ErrInvalidAutorun)
		}
		units := make([]uint16, 0, len(data)/2)
		for i := 0; i < len(data); i += 2 {
			units = append(units, uint16(data[i])|uint16(data[i+1])<<8)
		}
		return string(utf16.Decode(units)), nil
	}

	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%w: text není v kódování UTF-8 ani UTF-16", ErrInvalidAutorun)
	}
	return string(data), nil
}

// installerEntry přečte oddíl [AutoRun] a vrátí položku s instalátorem.
// Položky open a shellexecute jsou rovnocenné; pokud jsou uvedeny obě, musí
// být shodné.
func installerEntry(content string) (entry, command string, err error) {
	values, err := parseAutorunSection(content)
	if err != nil {
		return "", "", err
	}

	open, hasOpen := values["open"]
	shellExecute, hasShellExecute := values["shellexecute"]
	switch {
	case hasOpen && hasShellExecute && open != shellExecute:
		return "", "", fmt.Errorf("%w: položky open a shellexecute odkazují na různé programy", ErrInvalidAutorun)
	case hasOpen:
		entry, command = "open", open
	case hasShellExecute:
		entry, command = "shellexecute", shellExecute
	default:
		return "", "", ErrNoInstallerEntry
	}

	if command == "" {
		return "", "", fmt.Errorf("%w: položka %s je prázdná", ErrInvalidAutorun, entry)
	}
	return entry, command, nil
}

// parseAutorunSection vrátí položky oddílu [AutoRun] s klíči malými písmeny.
// Při opakování klíče platí první výskyt, stejně jako ve Windows.
func parseAutorunSection(content string) (map[string]string, error) {
	values := map[string]string{}
	foundSection := false
	inSection := false

	scanner := bufio.NewScanner(strings.NewReader(content))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("%w: řádek %d: neukončený název oddílu", ErrInvalidAutorun, lineNumber)
			}
			inSection = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), "autorun")
			foundSection = foundSection || inSection
			continue
		}
		if !inSection {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		if !ok || key == "" {
			return nil, fmt.Errorf("%w: řádek %d: očekáván zápis klíč=hodnota", ErrInvalidAutorun, lineNumber)
		}
		if _, exists := values[key]; !exists {
			values[key] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAutorun, err)
	}
	if !foundSection {
		return nil, fmt.Errorf("%w: chybí oddíl [AutoRun]", ErrInvalidAutorun)
	}
	return values, nil
}

// resolveInstallerPath ověří, že program je bezpečná relativní cesta uvnitř
// obrazu, a vrátí ji se skutečnou velikostí písmen ze svazku.
func resolveInstallerPath(fsys fs.FS, program string) (string, error) {
	segments, err := safePathSegments(program)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrUnsafeInstallerPath, program, err)
	}

	resolved := "."
	for _, segment := range segments {
		name, err := findEntry(fsys, resolved, segment)
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w: %q", ErrInstallerNotFound, program)
		}
		if err != nil {
			return "", fmt.Errorf("hledání instalátoru %q: %w", program, err)
		}
		resolved = name
	}

	info, err := fs.Stat(fsys, resolved)
	if err != nil {
		return "", fmt.Errorf("zjištění informací o instalátoru %q: %w", program, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %q není soubor", ErrInstallerNotFound, program)
	}
	return resolved, nil
}

// safePathSegments rozdělí cestu z autorun.inf na části a odmítne vše, co by
// mohlo vést mimo obraz: písmena jednotek, cesty UNC, proměnné prostředí,
// „..“, datové proudy a rezervované názvy Windows. Jedno úvodní lomítko
// znamená kořen obrazu.
func safePathSegments(program string) ([]string, error) {
	normalized := strings.ReplaceAll(program, `\`, "/")
	if strings.HasPrefix(normalized, "//") {
		return nil, errors.New("cesta UNC")
	}
	if strings.Contains(normalized, "%") {
		return nil, errors.New("proměnná prostředí")
	}
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "" {
		return nil, errors.New("prázdná cesta")
	}

	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		switch {
		case segment == "..":
			return nil, errors.New("odkaz na nadřazený adresář")
		case segment == "" || segment == ".":
			return nil, errors.New("prázdná část cesty")
		case !config.IsSafeInstallerFileName(segment):
			return nil, fmt.Errorf("nepovolený název %q", segment)
		}
	}
	return segments, nil
}

// findEntry vrátí cestu k položce adresáře dir, jejíž název odpovídá name bez
// ohledu na velikost písmen. Přesná shoda má přednost.
func findEntry(fsys fs.FS, dir, name string) (string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return "", err
	}
	match := ""
	for _, entry := range entries {
		if entry.Name() == name {
			return path.Join(dir, entry.Name()), nil
		}
		if match == "" && strings.EqualFold(entry.Name(), name) {
			match = path.Join(dir, entry.Name())
		}
	}
	if match == "" {
		return "", fs.ErrNotExist
	}
	return match, nil
}
