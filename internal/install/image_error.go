package install

import "fmt"

// ImageStage je krok instalace z obrazu disku, který selhal.
type ImageStage string

// Kroky instalace z obrazu disku, které končí chybou ImageError.
const (
	StageLock    ImageStage = "čekání na obraz disku"
	StageMount   ImageStage = "připojení obrazu disku"
	StageAutorun ImageStage = "ověření instalátoru v obrazu disku"
)

// ImageError popisuje selhání instalace z obrazu disku dříve, než se spustil
// instalátor. Příčinu lze dále zjistit pomocí errors.Is, například
// diskimage.ErrUnsupportedFormat nebo diskimage.ErrNoAutorun.
type ImageError struct {
	App   string
	Stage ImageStage
	Err   error
}

func (e *ImageError) Error() string {
	return fmt.Sprintf("%s aplikace %s: %v", e.Stage, e.App, e.Err)
}

func (e *ImageError) Unwrap() error {
	return e.Err
}
