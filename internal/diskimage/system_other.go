//go:build !windows

package diskimage

import "errors"

var errNotWindows = errors.New("připojování obrazů disků je podporováno jen ve Windows")

// otherSystem umožňuje sestavit balíček a spustit jeho testy i mimo Windows.
type otherSystem struct{}

func newSystem() system { return otherSystem{} }

func (otherSystem) openImage(string) (diskHandle, error)     { return 0, errNotWindows }
func (otherSystem) attachedDevice(diskHandle) (string, bool) { return "", false }
func (otherSystem) attach(diskHandle) error                  { return errNotWindows }
func (otherSystem) detach(diskHandle) error                  { return errNotWindows }
func (otherSystem) closeImage(diskHandle) error              { return nil }
func (otherSystem) volumeOf(string) (string, error)          { return "", errNotWindows }
