package install

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

// imageLocks zajišťuje, že stejný soubor obrazu používá v jednu chvíli jen
// jedna instalace. Jinak by jedna instalace mohla obraz odpojit, zatímco
// z něj druhá ještě instaluje, nebo by se obraz připojil dvakrát.
var imageLocks = struct {
	sync.Mutex
	byPath map[string]chan struct{}
}{byPath: map[string]chan struct{}{}}

// lockImage počká, až obraz na cestě path nepoužívá jiná instalace, a vrátí
// funkci pro jeho uvolnění. Čekání lze přerušit přes ctx.
func lockImage(ctx context.Context, path string) (unlock func(), err error) {
	// Windows nerozlišuje velikost písmen v cestách.
	key := strings.ToLower(filepath.Clean(path))

	imageLocks.Lock()
	slot, ok := imageLocks.byPath[key]
	if !ok {
		slot = make(chan struct{}, 1)
		imageLocks.byPath[key] = slot
	}
	imageLocks.Unlock()

	select {
	case slot <- struct{}{}:
		return func() { <-slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
