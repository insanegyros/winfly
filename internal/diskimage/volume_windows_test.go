package diskimage

import (
	"fmt"
	"syscall"
	"testing"
)

func TestSplitMultiString(t *testing.T) {
	buffer := append(syscall.StringToUTF16(`E:\`), syscall.StringToUTF16(`C:\mnt\obraz\`)...)
	buffer = append(buffer, 0, 0)
	got := fmt.Sprint(splitMultiString(buffer))
	if want := fmt.Sprint([]string{`E:\`, `C:\mnt\obraz\`}); got != want {
		t.Fatalf("splitMultiString() = %s, očekáváno %s", got, want)
	}
	if got := splitMultiString([]uint16{0, 0}); len(got) != 0 {
		t.Fatalf("prázdný seznam vrátil %v", got)
	}
}
