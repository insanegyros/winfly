# WinFly

WinFly je aplikace v jazyce Go určená výhradně pro Windows 10 a Windows 11. Používá přímé rozhraní Win32 bez webového rozhraní a bez externího GUI frameworku. Umožňuje načíst aktuální seznam aplikací ze vzdálené adresy, stáhnout vybrané instalátory, volitelně ověřit jejich SHA-256 a spustit je s oprávněním správce.

Aplikace také otevírá systémová okna Windows pro nastavení napájení a členství počítače v doméně nebo pracovní skupině. Tato nastavení sama nemění.

## Chování při spuštění

Při každém spuštění proběhne následující postup:

1. Aplikace načte pevnou konfiguraci vloženou při sestavení z `internal/config/config.json`.
2. Pokud neběží s oprávněním správce, vyvolá UAC a spustí se znovu se zvýšeným oprávněním.
3. Odešle požadavek `GET` na adresu uvedenou v `remoteConfigUrl`.
4. Přijaté tělo ověří jako JSON s povinným kořenovým polem `apps`.
5. Pokud je odpověď platná, zobrazí povolené aplikace v hlavním okně.
6. Pokud stažení nebo ověření selže, zobrazí chybový dialog, ponechá seznam skutečně prázdný a zakáže seznam, tlačítko **Vybrat vše** i tlačítko **Nainstalovat vybrané**.
7. Tlačítka pro nastavení Windows a okno protokolu zůstanou dostupná i při chybě vzdálené konfigurace.

Vzdálený seznam se záměrně neukládá jako záložní konfigurace. Každé spuštění požaduje aktuální odpověď a při chybě nepoužije starší data.

## Libovolný typ adresy koncového bodu

WinFly nekontroluje příponu cesty ani hodnotu hlavičky `Content-Type`. Rozhodující je pouze úspěšný stav HTTP a platný obsah JSON. Podporovány jsou například obě následující adresy:

```text
https://jandy.cz/config.json
https://jandy.cz/apps.php?get=applist
```

Koncový bod může být PHP, ASP.NET, API, reverzní proxy nebo statický soubor. Musí vrátit stav HTTP `2xx` a tělo ve správném formátu. Výstupní HTML, varování PHP, ladicí text před JSONem nebo více hodnot JSON způsobí odmítnutí celé odpovědi.

Aplikace přijme i UTF-8 BOM na začátku odpovědi. Neznámé vlastnosti jsou odmítnuty, aby překlepy v konfiguraci nezůstaly bez povšimnutí.

## Vložená konfigurace

Soubor `internal/config/config.json` se při sestavení vloží do `WinFly.exe`. Obsahuje stabilní nastavení aplikace a adresu vzdáleného seznamu, nikoli samotný seznam aplikací.

```json
{
  "productName": "WinFly",
  "cacheDirectory": "${ProgramData}\\WinFly\\cache",
  "remoteConfigUrl": "https://jandy.cz/apps.php?get=applist",
  "network": {
    "configTimeoutSeconds": 30,
    "maxConfigBytes": 1048576,
    "timeoutSeconds": 900,
    "maxDownloadBytes": 8589934592,
    "allowHttp": false
  }
}
```

### Vlastnosti vložené konfigurace

| Vlastnost | Význam |
|---|---|
| `productName` | Název zobrazený v oknech aplikace. Výchozí hodnota je `WinFly`. |
| `cacheDirectory` | Adresář pro stažené instalátory. Proměnné prostředí Windows se rozbalí za běhu. |
| `remoteConfigUrl` | Povinná absolutní adresa vracející vzdálený JSON. Může obsahovat libovolnou cestu a parametry dotazu. |
| `network.configTimeoutSeconds` | Celkový časový limit pro načtení vzdálené konfigurace. Výchozí hodnota je 30 sekund. |
| `network.maxConfigBytes` | Nejvyšší velikost vzdálené konfigurace. Výchozí hodnota je 1 MiB. |
| `network.timeoutSeconds` | Celkový časový limit pro stažení jednoho instalátoru. Výchozí hodnota je 900 sekund. |
| `network.maxDownloadBytes` | Nejvyšší velikost jednoho instalátoru. Výchozí hodnota je 8 GiB. |
| `network.allowHttp` | Povolí nešifrované HTTP pro konfiguraci i instalátory. Výchozí hodnota `false` vyžaduje HTTPS. |

Schéma této konfigurace je v `docs/config.schema.json`.

## Formát vzdáleného JSONu

Odpověď musí být jeden kořenový objekt s povinným polem `apps`. Prázdné pole je platné a pouze způsobí prázdný zakázaný seznam bez chybového dialogu.

```json
{
  "apps": [
    {
      "id": "wf-ukazkova-aplikace",
      "name": "Ukázková aplikace WinFly",
      "description": "Popis zobrazený v přehledu aplikace.",
      "enabled": true,
      "url": "https://jandy.cz/installer.exe",
      "sha256": "",
      "fileName": "installer.exe",
      "installerType": "exe",
      "arguments": ["/quiet", "/norestart"],
      "wait": true,
      "successExitCodes": [0, 1641, 3010]
    }
  ]
}
```

Úplný příklad je v `docs/apps.example.json`. Schéma odpovědi je v `docs/apps.schema.json`. Soubor `docs/apps.example.php` ukazuje jednoduchý koncový bod PHP.

### Vlastnosti aplikace

| Vlastnost | Význam |
|---|---|
| `id` | Povinný jedinečný technický identifikátor. Duplicitní hodnoty zneplatní celou odpověď. |
| `name` | Název v seznamu, přehledu a protokolu. U povolené aplikace je povinný. |
| `description` | Volitelný popis v přehledu aplikace. |
| `enabled` | Pouze povolené položky se zobrazí, stáhnou a spustí. Zakázaná položka může sloužit jako neúplná šablona. |
| `url` | Absolutní URL instalátoru. HTTPS je povinné, pokud vložená konfigurace nepovolí HTTP. |
| `sha256` | Volitelný otisk. Přesně 64 hexadecimálních znaků zapne ověřování; chybějící nebo neplatná hodnota zobrazí křížek a ověřování vypne. |
| `fileName` | Bezpečný základní název souboru Windows bez cesty, datového proudu nebo rezervovaného názvu. |
| `installerType` | `exe`, `msi`, `iso`, `img` nebo `auto` pro určení podle přípony `fileName` (velikost písmen nerozhoduje). |
| `arguments` | Pole parametrů předaných instalátoru přímo, bez `cmd.exe` nebo PowerShellu. |
| `wait` | Čekání na ukončení instalátoru a kontrola návratového kódu. Výchozí hodnota je `true`; MSI, ISO a IMG vyžadují `true`. |
| `successExitCodes` | Přijímané návratové kódy. Výchozí hodnoty jsou `0`, `1641` a `3010`. |

Pokud je jediná povolená položka neplatná, je neplatná celá vzdálená konfigurace. Aplikace poté zobrazí chybu a nepovolí instalaci žádné položky.

## Ověřování SHA-256

Pro produkční použití je vhodné uvádět SHA-256 získaný z důvěryhodného zdroje dodavatele:

```powershell
Get-FileHash .\installer.exe -Algorithm SHA256
```

Při platném otisku WinFly:

1. použije soubor z mezipaměti pouze tehdy, když jeho otisk souhlasí;
2. počítá SHA-256 během zápisu stahovaných dat do dočasného souboru;
3. přesune soubor na cílové místo až po úspěšném ověření;
4. bezprostředně před spuštěním otevře přesně tento soubor bez sdílení pro zápis nebo odstranění a znovu jej ověří.

Při chybějícím nebo neplatném otisku instalace zůstává dostupná, ale přehled zobrazuje křížek, protokol zapisuje upozornění, neověřený soubor z mezipaměti se znovu nepoužije a nelze tvrdit kryptografickou integritu instalátoru.

## Instalace z obrazu disku ISO/IMG

Položka s `installerType` `iso` nebo `img` (nebo `auto` a příponou `.iso`/`.img`) se stáhne a ověří stejně jako EXE a MSI. Poté WinFly:

1. ověří podle obsahu, že jde o obraz ISO 9660 nebo UDF (obraz pevného disku nebo nepodporovaný formát odmítne);
2. připojí obraz pouze pro čtení nativně přes `virtdisk.dll`, bez PowerShellu a programů třetích stran;
3. v kořeni obrazu vyhledá `autorun.inf` a v sekci `[AutoRun]` položku `open=` nebo `shellexecute=`; cílem musí být existující soubor `.exe` nebo `.msi` uvnitř obrazu (bez `..` a písmen jednotek), jinak obraz odmítne; `setup.exe` se nehádá;
4. spustí instalátor stejně jako samostatný EXE nebo MSI: nejprve parametry z `autorun.inf`, za nimi `arguments` z konfigurace, pracovním adresářem je kořen obrazu; žádné parametry pro tichou instalaci se nepřidávají;
5. počká na ukončení instalátoru a obraz odpojí, i po chybě nebo zrušení. Obraz připojený dříve uživatelem zůstane připojený.

V přehledu aplikace řádek „Typ instalátoru“ ukazuje stav obrazu: neověřeno, ověřeno, nebo neplatný. Neplatný obraz (chybí `autorun.inf`, neplatný `autorun.inf`, nepodporovaný formát) se do restartu WinFly nebo do změny položky v seznamu znovu nestahuje ani nespouští.

Omezení: WinFly čeká jen na proces spuštěný z `autorun.inf`. Pokud ten spustí jiný instalátor a hned skončí, obraz se odpojí dříve; pro takový instalátor uveďte v `arguments` jeho vlastní parametr pro čekání. Webový panel (`admin_panel_web/`) umí obrazy `.iso`/`.img` nahrát i upravit.

## Stahování a spuštění instalátorů

Instalátory nejsou součástí `WinFly.exe`. Po stisknutí tlačítka **Nainstalovat vybrané** se stáhnou z adres uvedených v právě načteném vzdáleném seznamu.

- EXE se spustí přímo s uvedeným polem parametrů.
- MSI se spustí prostřednictvím `msiexec.exe /i`.
- Nepoužívá se příkazový interpret.
- Stavové kódy `1641` a `3010` označí požadavek na restart Windows.
- Mezipaměť je zabezpečena pomocí `icacls`, aby do ní měli úplný přístup pouze LocalSystem a místní správci.

## Systémová okna Windows

Tlačítko **Otevřít nastavení napájení** spustí:

```text
control.exe /name Microsoft.PowerOptions /page pageGlobalSettings
```

Tlačítko **Otevřít doménu / skupinu** spustí:

```text
SystemPropertiesComputerName.exe
```

Následné změny, požadavky na přihlašovací údaje, zásady, členství v doméně, pracovní skupina a případný restart jsou plně v režii Windows.

## Protokol

Hlavní okno neobsahuje vložený panel protokolu. Tlačítko **Zobrazit protokol** otevře samostatné měnitelné okno a tlačítko se změní na **Skrýt protokol**. Zavření okna protokolu jej pouze skryje; hlavní aplikace pokračuje v běhu a další záznamy se neztratí.

## Sestavení

Požadavky:

- Windows 10 nebo Windows 11;
- Go 1.23 nebo novější;
- PowerShell;
- přístup k síti pro test vzdáleného koncového bodu až při běhu aplikace, nikoli při sestavení.

Upravte `internal/config/config.json` a spusťte:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\build.ps1
```

Jiná architektura:

```powershell
.\build.ps1 -Architecture arm64
.\build.ps1 -Architecture 386
```

Výstup je uložen v `dist`. Skript provede `go test ./...`, `go vet ./...` a sestaví GUI aplikaci s parametrem `-H=windowsgui`.

### Manifest aplikace

Soubory `cmd/winfly/rsrc_windows_*.syso` obsahují manifest Windows, který zapíná moderní vzhled ovládacích prvků (Common Controls 6) a podporu vysokého DPI. `go build` je do programu přidá automaticky. Po úpravě `cmd/winfly/winres/winres.json` je vygenerujte znovu:

```powershell
go install github.com/tc-hib/go-winres@latest
cd cmd\winfly
go-winres make --arch amd64,386,arm64
```

## Bezpečnost vzdálené konfigurace

Vzdálený koncový bod je privilegovaná hranice důvěry. Jeho odpověď může určit, které soubory aplikace stáhne a spustí s oprávněním správce. Kompromitovaný server může proto vést ke spuštění škodlivého instalátoru, zejména pokud položky nemají platný SHA-256.

Pro produkční provoz:

- používejte HTTPS a ponechte `allowHttp` na hodnotě `false`;
- chraňte server, DNS, TLS certifikáty a proces publikování seznamu;
- uvádějte důvěryhodné SHA-256 pro každou aplikaci;
- zvažte navíc kontrolu podpisu Authenticode;
- nepředávejte v URL, konfiguraci ani parametrech instalační tajné údaje;
- podepište `WinFly.exe` firemním certifikátem;
- první spuštění ověřte v jednorázovém virtuálním stroji Windows 10 a Windows 11.

Další podrobnosti jsou v `SECURITY.md`.

## Struktura projektu

```text
cmd/winfly/       Vstupní bod aplikace Windows
internal/admin/           Zvýšení oprávnění UAC a chybový dialog
internal/config/          Vložená konfigurace, vzdálené načítání a validace JSON
internal/download/        Stahování instalátorů, limity, mezipaměť a SHA-256
internal/install/         Spuštění EXE/MSI, uzamčení souboru a návratové kódy
internal/systemdialogs/   Otevření Možností napájení a Vlastností systému
internal/ui/              Okna aplikace (hlavní, protokol, systémové informace) přes Win32
internal/ui/win32/        Tenká vrstva nad voláními Win32 API používaná oknem aplikace
docs/                     Schémata, ukázkový JSON a ukázkový koncový bod PHP
admin_panel_web/          Webový panel pro správu apps.json (čisté PHP)
build.ps1                 Testy, statická kontrola a sestavení GUI aplikace
```

## Webový panel

Složka `admin_panel_web/` je administrace, která spravuje `apps.json` a nahrává instalátory EXE, MSI, ISO a IMG. Je napsaná v čistém PHP (7.4 nebo novější) s Bootstrapem a jQuery uloženými přímo ve složce `assets/vendor/`. Nepotřebuje Composer, npm, databázi ani přístup do konzole, stačí ji nahrát na běžný webhosting.

- Nastavení (heslo, cesta k `apps.json`, složka a veřejná URL instalátorů) je v `config.php`.
- Velké soubory se nahrávají po blocích, takže limity `upload_max_filesize` a `post_max_size` jejich velikost neomezují.
- Panel ukládá jen záznamy, které WinFly přijme: typ instalátoru musí odpovídat souboru a MSI, ISO a IMG mají vždy zapnuté čekání na dokončení.

Struktura: `index.php` (přihlášení a stránka), `api.php` (JSON API pro skripty stránky), `src/` (logika PHP), `views/` (šablony HTML), `assets/js/` (skripty stránky). Složky `src/` a `views/` nejsou určené k přímému přístupu z prohlížeče.

Při aktualizaci starší verze panelu na serveru smažte soubory `ProvisionerJsonCRUD.php`, `ProvisionerChunkUpload.php`, `assets/admin.js` a složku `.provisioner-uploads`, pokud existuje. Nový `config.php` obsahuje heslo; pokud ponecháváte vlastní starší `config.php`, doplňte do něj položku `adminPassword`.

## Ověření před nasazením

Nativní GUI, UAC, `icacls`, stahování přes firemní proxy, spouštění instalátorů, Ovládací panely a Vlastnosti systému je nutné ověřit pod zásadami používanými v cílovém prostředí Windows. Zvláštní pozornost věnujte chování při nedostupném serveru, neplatném certifikátu, odpovědi HTTP mimo rozsah `2xx`, neplatném JSONu, příliš velké odpovědi a nesouhlasícím SHA-256.
