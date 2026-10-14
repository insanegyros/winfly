package install

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"jandy.cz/winfly/internal/config"
	"jandy.cz/winfly/internal/diskimage"
)

func imageApp() config.AppConfig {
	return config.AppConfig{
		ID:            7,
		Name:          "Office",
		URL:           "https://example.com/office.iso",
		FileName:      "office.iso",
		InstallerType: "auto",
	}
}

func imageError(stage ImageStage, cause error) error {
	return &ImageError{App: "Office", Stage: stage, Err: fmt.Errorf("%w: podrobnosti", cause)}
}

func TestImageStatusesRecord(t *testing.T) {
	tests := []struct {
		name        string
		result      Result
		err         error
		wantStatus  ImageStatus
		wantMessage string
	}{
		{name: "úspěšná instalace", result: Result{ImageValidated: true}, wantStatus: ImageInstallable},
		{name: "instalátor selhal po ověření", result: Result{ImageValidated: true, ExitCode: 5}, err: errors.New("kód 5"), wantStatus: ImageInstallable},
		{name: "chybí autorun.inf", err: imageError(StageAutorun, diskimage.ErrNoAutorun), wantStatus: ImageInvalid, wantMessage: MessageNoAutorun},
		{name: "neplatný autorun.inf", err: imageError(StageAutorun, diskimage.ErrInvalidAutorun), wantStatus: ImageInvalid, wantMessage: MessageInvalidAutorun},
		{name: "jen ikona", err: imageError(StageAutorun, diskimage.ErrNoInstallerEntry), wantStatus: ImageInvalid, wantMessage: MessageInvalidAutorun},
		{name: "nebezpečná cesta", err: imageError(StageAutorun, diskimage.ErrUnsafeInstallerPath), wantStatus: ImageInvalid, wantMessage: MessageInvalidAutorun},
		{name: "chybí instalátor", err: imageError(StageAutorun, diskimage.ErrInstallerNotFound), wantStatus: ImageInvalid, wantMessage: MessageInvalidAutorun},
		{name: "nepodporovaný formát", err: imageError(StageMount, diskimage.ErrUnsupportedFormat), wantStatus: ImageInvalid, wantMessage: MessageUnsupportedFormat},
		// Přechodné chyby obraz neoznačí jako neplatný.
		{name: "chybí oprávnění", err: imageError(StageMount, diskimage.ErrAccessDenied), wantStatus: ImageUnverified},
		{name: "chybí svazek", err: imageError(StageMount, diskimage.ErrNoVolume), wantStatus: ImageUnverified},
		{name: "chyba před připojením", err: errors.New("instalátor se po ověření stažení změnil"), wantStatus: ImageUnverified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statuses := NewImageStatuses()
			statuses.Record(imageApp(), tt.result, tt.err)
			status, message, ok := statuses.Status(imageApp())
			if !ok || status != tt.wantStatus || message != tt.wantMessage {
				t.Fatalf("Status() = %v %q %t, očekáváno %v %q", status, message, ok, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}

func TestImageStatusesBlockInvalidImageUntilItChanges(t *testing.T) {
	statuses := NewImageStatuses()
	app := imageApp()
	if err := statuses.CheckAllowed(app); err != nil {
		t.Fatalf("neověřený obraz musí jít nainstalovat: %v", err)
	}

	statuses.Record(app, Result{}, imageError(StageAutorun, diskimage.ErrNoAutorun))
	err := statuses.CheckAllowed(app)
	var rejected *RejectedImageError
	if !errors.As(err, &rejected) {
		t.Fatalf("CheckAllowed() = %v, očekávána RejectedImageError", err)
	}
	if UserMessage(err) != MessageNoAutorun {
		t.Fatalf("UserMessage() = %q", UserMessage(err))
	}

	// Jiný obraz (nový otisk v seznamu aplikací) je znovu neověřený.
	changed := app
	changed.SHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if status, _, _ := statuses.Status(changed); status != ImageUnverified {
		t.Fatalf("změněný obraz má stav %v, očekáváno neověřeno", status)
	}
	if err := statuses.CheckAllowed(changed); err != nil {
		t.Fatalf("změněný obraz musí jít nainstalovat: %v", err)
	}

	// Úspěšné ověření obraz opět povolí.
	statuses.Record(app, Result{ImageValidated: true}, nil)
	if err := statuses.CheckAllowed(app); err != nil {
		t.Fatalf("znovu ověřený obraz musí jít nainstalovat: %v", err)
	}
}

func TestImageStatusesIgnoreEXEAndMSI(t *testing.T) {
	statuses := NewImageStatuses()
	for _, fileName := range []string{"setup.exe", "setup.msi"} {
		app := config.AppConfig{ID: 1, Name: "App", FileName: fileName, InstallerType: "auto"}
		statuses.Record(app, Result{}, imageError(StageAutorun, diskimage.ErrNoAutorun))
		if _, _, ok := statuses.Status(app); ok {
			t.Errorf("%s: Status() hlásí obraz disku", fileName)
		}
		if err := statuses.CheckAllowed(app); err != nil {
			t.Errorf("%s: CheckAllowed() = %v", fileName, err)
		}
	}
}

func TestUserMessage(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: imageError(StageAutorun, diskimage.ErrNoAutorun), want: MessageNoAutorun},
		{err: imageError(StageAutorun, diskimage.ErrInstallerNotFound), want: MessageInvalidAutorun},
		{err: imageError(StageMount, diskimage.ErrUnsupportedFormat), want: MessageUnsupportedFormat},
		{err: imageError(StageMount, diskimage.ErrAccessDenied), want: messageMountFailed},
		{err: imageError(StageLock, context.Canceled), want: messageImageFailed},
		{err: errors.New("aplikace X skončila s kódem 5"), want: "aplikace X skončila s kódem 5"},
	}
	for _, tt := range tests {
		if got := UserMessage(tt.err); got != tt.want {
			t.Errorf("UserMessage(%v) = %q, očekáváno %q", tt.err, got, tt.want)
		}
	}
}
