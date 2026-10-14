# Bezpečnostní poznámky

## Hranice důvěry

WinFly běží se zvýšeným oprávněním a spouští instalátory určené vzdálenou konfigurací. Za privilegované změny vyžadující kontrolu považujte zejména:

- `internal/config/config.json` a hodnotu `remoteConfigUrl`;
- obsah publikovaný vzdáleným koncovým bodem;
- URL instalátorů, SHA-256, parametry, přijímané návratové kódy a příznak `wait`;
- sestavovací skripty a výsledné vydané soubory.

Kompromitování vzdáleného koncového bodu může vést ke spuštění libovolných binárních dat s oprávněním správce. Samotné HTTPS chrání přenos, ale nenahrazuje ochranu serveru, procesu publikování ani kontrolu instalátorů.

## Bezpečné selhání vzdáleného seznamu

Při každém spuštění se seznam stáhne znovu. Aplikace nepoužívá trvalou záložní kopii ani starší úspěšnou odpověď.

Následující situace zneplatní vzdálenou konfiguraci:

- chyba DNS, TLS, proxy, časový limit nebo jiná síťová chyba;
- stav HTTP mimo rozsah `2xx`;
- odpověď větší než `network.maxConfigBytes`;
- neplatný JSON, více hodnot JSON nebo chybějící pole `apps`;
- neznámé vlastnosti;
- duplicitní identifikátory aplikací;
- neplatná povolená aplikace, například nebezpečný `fileName`, zakázané HTTP nebo neplatný typ instalátoru.

Při takové chybě se zobrazí chybový dialog, seznam aplikací zůstane prázdný a instalační ovládací prvky se zakážou. Ostatní systémová tlačítka zůstávají dostupná.

Přípona URL a `Content-Type` se záměrně nekontrolují, aby mohl odpověď poskytovat například PHP nebo jiné API. Tělo odpovědi však musí být samostatný platný dokument JSON. Výstupní HTML, varování serverového skriptu nebo ladicí text způsobí bezpečné odmítnutí.

## Integrita instalátorů

SHA-256 je volitelný.

Pokud aplikace obsahuje platný 64znakový hexadecimální SHA-256, stahovaná data se zapisují do dočasného souboru s limitem velikosti a současně se počítá otisk. Soubor se přesune na cílové místo pouze při shodě. Existující soubor v mezipaměti se použije pouze po novém ověření. Bezprostředně před vytvořením procesu se instalátor otevře bez sdílení pro zápis nebo odstranění, znovu se ověří přesně tento otevřený soubor a popisovač zůstane otevřený během spuštění.

Pokud SHA-256 chybí nebo je neplatný, přehled zobrazí křížek, protokol zapíše upozornění, neověřený soubor z mezipaměti se znovu nepoužije a stáhne se čerstvá kopie. Soubor zůstává před spuštěním uzamčený proti zápisu nebo odstranění, ale jeho vydavatel ani obsah nejsou kryptograficky ověřeny. Tento režim má výrazně nižší úroveň jistoty.

Adresář mezipaměti je pomocí `icacls` nastaven tak, aby úplné řízení získal pouze LocalSystem a místní skupina Administrators. Pokud nastavení oprávnění selže, inicializace správce stahování selže.

SHA-256 potvrzuje shodu bajtů, nikoli totožnost vydavatele. V produkčním prostředí je vhodné doplnit kontrolu podpisu Authenticode nebo porovnat otisk se softwarovým katalogem chráněným odděleným autentizovaným kanálem.

## HTTP, přesměrování a limity

`network.allowHttp` je ve výchozím nastavení `false`. Toto pravidlo platí pro vzdálenou konfiguraci i URL instalátorů. Při zakázaném HTTP se odmítne také přesměrování z HTTPS na HTTP. Počet přesměrování je omezen.

Vzdálená konfigurace má samostatný časový limit a limit velikosti. Instalátory mají delší samostatný časový limit a vlastní limit velikosti. Limity se uplatní i tehdy, když server neuvede `Content-Length`.

## Parametry instalátoru

Parametry se předávají přímo podřízenému procesu, nikoli prostřednictvím `cmd.exe` nebo PowerShellu. Stále jde o důvěryhodnou konfiguraci, která může zásadně měnit chování instalátoru. Kontrolujte dokumentaci dodavatele a nepoužívejte parametry vypínající ověřování podpisů, TLS nebo ochranu koncových bodů.

Do vložené ani vzdálené konfigurace, URL a parametrů nevkládejte hesla, přístupové tokeny, licenční klíče ani jiné tajné údaje. Spustitelný soubor, síťový provoz, protokoly a konfiguraci lze kontrolovat.

## Systémová nastavení a členství v doméně

WinFly nezapisuje nastavení napájení, nemanipuluje s rychlým spuštěním, neshromažďuje přihlašovací údaje k doméně a nevolá rozhraní pro přímé připojení k doméně. Pouze otevírá rozhraní vlastněná Windows:

- Možnosti napájení v Ovládacích panelech;
- stránku Název počítače ve Vlastnostech systému pro správu domény nebo pracovní skupiny.

Veškerou následnou autorizaci, práci s přihlašovacími údaji, kontrolu zásad, změny a výzvy k restartu provádí Windows.

## Provozní opatření

První spuštění proveďte v jednorázovém virtuálním stroji Windows 10 a Windows 11. Ověřte chování při firemní proxy, filtrování TLS, UAC, antiviru, EDR a zásadách řízení aplikací. Ověřte také všechny chybové scénáře vzdálené konfigurace a návratové kódy instalátorů.

Výsledný `WinFly.exe` podepište a jeho kontrolní součet publikujte samostatným autentizovaným kanálem.
