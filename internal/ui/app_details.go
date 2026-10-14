package ui

import (
	"strings"

	"jandy.cz/winfly/internal/config"
	"jandy.cz/winfly/internal/install"
)

// appDetails is the "Přehled aplikace" panel of the main window. It shows
// everything known about the app selected in the list.
type appDetails struct {
	name        control
	description control
	url         control
	arguments   control
	checksum    control
	kind        control
}

func newAppDetails(f *form, b box) appDetails {
	rows := f.section("Přehled aplikace", b, 140)
	return appDetails{
		name:        rows.field("Název:"),
		description: rows.textArea("Popis:", 6),
		url:         rows.field("Adresa pro stažení:"),
		arguments:   rows.textArea("Parametry instalátoru:", 3),
		checksum:    rows.text("Platnost SHA-256:"),
		kind:        rows.text("Typ instalátoru:"),
	}
}

// show fills the panel with app. kindText is from installerTypeText.
func (d appDetails) show(app config.AppConfig, kindText string) {
	d.name.setText(app.Name)
	d.description.setText(orNotSpecified(app.Description))
	d.url.setText(app.URL)
	d.arguments.setText(orNotSpecified(strings.Join(app.Arguments, " ")))
	if app.HasValidSHA256() {
		d.checksum.setText("✓ Platný — ověřování je zapnuto")
	} else {
		d.checksum.setText("✗ Chybí nebo je neplatný — ověřování je vypnuto")
	}
	d.kind.setText(kindText)
}

// installerTypeText describes the installer type, and for ISO/IMG images
// also whether the image passed validation.
func installerTypeText(app config.AppConfig, statuses *install.ImageStatuses) string {
	kind, err := config.ResolveInstallerType(app.InstallerType, app.FileName)
	if err != nil {
		return "Neznámý — nelze určit z názvu souboru"
	}
	status, _, isImage := statuses.Status(app)
	if !isImage {
		return strings.ToUpper(kind)
	}
	return "Obraz disku " + strings.ToUpper(kind) + " — " + status.String()
}

// showNoApps explains why the app list is empty.
func (d appDetails) showNoApps(appListErr error) {
	if appListErr != nil {
		d.clear("Seznam aplikací není k dispozici")
		d.description.setText("Načtení vzdálené konfigurace selhalo. Instalace aplikací je zakázána.")
	} else {
		d.clear("Žádné dostupné aplikace")
		d.description.setText("Vzdálená konfigurace neobsahuje žádnou povolenou aplikaci.")
	}
}

// clear empties the panel and puts a short message where the name goes.
func (d appDetails) clear(message string) {
	d.name.setText(message)
	d.description.setText("")
	d.url.setText("")
	d.arguments.setText("")
	d.checksum.setText("—")
	d.kind.setText("—")
}

func orNotSpecified(value string) string {
	if value == "" {
		return "(neuvedeno)"
	}
	return value
}
