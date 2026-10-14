package install

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLockImageAllowsOneUserPerImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Office.iso")
	unlock, err := lockImage(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	// Stejný obraz zapsaný jinou velikostí písmen je stále obsazený.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := lockImage(ctx, filepath.Join(filepath.Dir(path), "OFFICE.ISO")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("druhé uzamčení = %v, očekáváno čekání do vypršení", err)
	}

	// Jiný obraz lze uzamknout hned.
	other, err := lockImage(context.Background(), filepath.Join(t.TempDir(), "jiny.iso"))
	if err != nil {
		t.Fatal(err)
	}
	other()

	unlock()
	again, err := lockImage(context.Background(), path)
	if err != nil {
		t.Fatalf("uvolněný obraz nejde znovu uzamknout: %v", err)
	}
	again()
}
