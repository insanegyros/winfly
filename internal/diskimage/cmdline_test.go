package diskimage

import (
	"errors"
	"fmt"
	"testing"
)

func TestSplitCommandLine(t *testing.T) {
	tests := []struct {
		command     string
		wantProgram string
		wantArgs    []string
	}{
		{command: "setup.exe", wantProgram: "setup.exe", wantArgs: []string{}},
		{command: `setup.exe /q /norestart`, wantProgram: "setup.exe", wantArgs: []string{"/q", "/norestart"}},
		{command: "setup.exe\t/q", wantProgram: "setup.exe", wantArgs: []string{"/q"}},
		{command: `"My Setup.exe"`, wantProgram: "My Setup.exe", wantArgs: []string{}},
		{command: `"x64\Instalace programu.exe" /lang cs`, wantProgram: `x64\Instalace programu.exe`, wantArgs: []string{"/lang", "cs"}},
		{command: `setup.exe "/dir=C:\Program Files\App" /q`, wantProgram: "setup.exe", wantArgs: []string{`/dir=C:\Program Files\App`, "/q"}},
		{command: `setup.exe /title="Žluťoučký kůň"`, wantProgram: "setup.exe", wantArgs: []string{"/title=Žluťoučký kůň"}},
		{command: `setup.exe a\\\"b c\\"d e"`, wantProgram: "setup.exe", wantArgs: []string{`a\"b`, `c\d e`}},
		{command: `setup.exe ""`, wantProgram: "setup.exe", wantArgs: []string{""}},
		{command: `setup.exe C:\temp\`, wantProgram: "setup.exe", wantArgs: []string{`C:\temp\`}},
		{command: `  setup.exe   /a   /b  `, wantProgram: "setup.exe", wantArgs: []string{"/a", "/b"}},
	}
	for _, tt := range tests {
		program, args, err := splitCommandLine(tt.command)
		if err != nil {
			t.Errorf("splitCommandLine(%q) vrátil chybu: %v", tt.command, err)
			continue
		}
		if program != tt.wantProgram || fmt.Sprintf("%q", args) != fmt.Sprintf("%q", tt.wantArgs) {
			t.Errorf("splitCommandLine(%q) = %q %q, očekáváno %q %q", tt.command, program, args, tt.wantProgram, tt.wantArgs)
		}
	}
}

func TestSplitCommandLineRejectsUnterminatedQuotes(t *testing.T) {
	for _, command := range []string{`"setup.exe`, `setup.exe "/dir=C:\x`, `setup.exe \\"a`} {
		if _, _, err := splitCommandLine(command); !errors.Is(err, errUnterminatedQuote) {
			t.Errorf("splitCommandLine(%q) = %v, očekávány neukončené uvozovky", command, err)
		}
	}
}
