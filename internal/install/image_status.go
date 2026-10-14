package install

import (
	"errors"
	"fmt"
	"sync"

	"jandy.cz/winfly/internal/config"
	"jandy.cz/winfly/internal/diskimage"
)

// Texty pro uživatele u neplatných obrazů disků. Technické podrobnosti se
// zapisují do logu.
const (
	MessageNoAutorun         = "Neplatný instalační obraz: v kořenovém adresáři chybí autorun.inf. Odeberte tento soubor ze seznamu instalátorů."
	MessageInvalidAutorun    = "Neplatná konfigurace autorun.inf nebo instalační soubor nebyl nalezen."
	MessageUnsupportedFormat = "Nepodporovaný formát obrazu disku. Soubor nelze připojit."

	// Přechodné chyby, po kterých obraz zůstává neověřený.
	messageMountFailed = "Obraz disku se nepodařilo připojit. Podrobnosti najdete v logu."
	messageImageFailed = "Instalace z obrazu disku se nezdařila. Podrobnosti najdete v logu."
)

// RejectedImageError vrací CheckAllowed pro obraz, který dříve neprošel
// ověřením, a proto se znovu nestahuje ani nespouští.
type RejectedImageError struct {
	App     string
	Message string // text pro uživatele z doby, kdy obraz ověřením neprošel
}

func (e *RejectedImageError) Error() string {
	return fmt.Sprintf("aplikace %s se nespustí, obraz disku dříve neprošel ověřením: %s", e.App, e.Message)
}

// ImageStatus je stav ověření obrazu disku během běhu programu.
type ImageStatus int

const (
	// ImageUnverified: obraz ještě nebyl ověřen; ověří se před instalací.
	ImageUnverified ImageStatus = iota
	// ImageInstallable: obraz šel připojit a jeho autorun.inf je platný.
	ImageInstallable
	// ImageInvalid: obraz neprošel ověřením a jeho instalace je zakázána.
	ImageInvalid
)

func (s ImageStatus) String() string {
	switch s {
	case ImageInstallable:
		return "ověřeno, lze instalovat"
	case ImageInvalid:
		return "neplatný, instalace je zakázána"
	default:
		return "neověřeno, ověří se před instalací"
	}
}

// imageKey určuje obraz. Pokud se v seznamu aplikací změní adresa, otisk
// nebo název souboru, jde o jiný obraz a ten je opět neověřený.
type imageKey struct {
	id       int64
	url      string
	sha256   string
	fileName string
}

func keyOf(app config.AppConfig) imageKey {
	return imageKey{
		id:       app.ID,
		url:      app.URL,
		sha256:   app.SHA256,
		fileName: app.FileName,
	}
}

type imageRecord struct {
	status  ImageStatus
	message string // text pro uživatele u neplatného obrazu
}

// ImageStatuses si pamatuje výsledky ověření obrazů disků. Je bezpečné ho
// používat z více goroutin.
type ImageStatuses struct {
	mu      sync.Mutex
	records map[imageKey]imageRecord
}

// NewImageStatuses vrátí prázdný přehled; všechny obrazy jsou neověřené.
func NewImageStatuses() *ImageStatuses {
	return &ImageStatuses{records: map[imageKey]imageRecord{}}
}

// Status vrátí stav obrazu aplikace a u neplatného obrazu i text pro
// uživatele. Pro aplikace, které nejsou obrazem disku, vrátí ok=false.
func (s *ImageStatuses) Status(app config.AppConfig) (status ImageStatus, message string, ok bool) {
	if !isImageApp(app) {
		return ImageUnverified, "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[keyOf(app)]
	return record.status, record.message, true
}

// CheckAllowed vrátí chybu, pokud obraz aplikace dříve neprošel ověřením.
// Volá se před stažením, aby se neplatný obraz znovu nespouštěl.
func (s *ImageStatuses) CheckAllowed(app config.AppConfig) error {
	status, message, ok := s.Status(app)
	if !ok || status != ImageInvalid {
		return nil
	}
	return &RejectedImageError{App: app.Name, Message: message}
}

// Record uloží výsledek install.Run pro obraz disku. Obraz je platný, pokud
// prošel ověřením autorun.inf (i když instalátor pak skončil chybou), a
// neplatný, pokud má nepodporovaný formát nebo neplatný autorun.inf. Jiné
// chyby (síť, oprávnění, zrušení) stav nemění.
func (s *ImageStatuses) Record(app config.AppConfig, result Result, runErr error) {
	if !isImageApp(app) {
		return
	}
	var record imageRecord
	var imageErr *ImageError
	switch {
	case result.ImageValidated:
		record = imageRecord{status: ImageInstallable}
	case errors.As(runErr, &imageErr) && imageUserMessage(imageErr) != "":
		record = imageRecord{status: ImageInvalid, message: imageUserMessage(imageErr)}
	default:
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[keyOf(app)] = record
}

// UserMessage vrátí srozumitelný text chyby pro dialog. U chyb obrazu disku
// použije pevné texty výše, jinak vrátí text chyby beze změny.
func UserMessage(err error) string {
	var rejectedErr *RejectedImageError
	if errors.As(err, &rejectedErr) {
		return rejectedErr.Message
	}
	var imageErr *ImageError
	if !errors.As(err, &imageErr) {
		return err.Error()
	}
	if message := imageUserMessage(imageErr); message != "" {
		return message
	}
	if imageErr.Stage == StageMount {
		return messageMountFailed
	}
	return messageImageFailed
}

// imageUserMessage vrátí text pro uživatele, pokud chyba znamená, že obraz
// sám je neplatný. Pro přechodné chyby vrátí prázdný řetězec.
func imageUserMessage(err *ImageError) string {
	switch {
	case errors.Is(err, diskimage.ErrNoAutorun):
		return MessageNoAutorun
	case errors.Is(err, diskimage.ErrInvalidAutorun),
		errors.Is(err, diskimage.ErrNoInstallerEntry),
		errors.Is(err, diskimage.ErrUnsafeInstallerPath),
		errors.Is(err, diskimage.ErrInstallerNotFound):
		return MessageInvalidAutorun
	case errors.Is(err, diskimage.ErrUnsupportedFormat):
		return MessageUnsupportedFormat
	default:
		return ""
	}
}

func isImageApp(app config.AppConfig) bool {
	kind, err := config.ResolveInstallerType(app.InstallerType, app.FileName)
	return err == nil && config.IsDiskImageType(kind)
}
