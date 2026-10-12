<?php

declare(strict_types=1);

/*
 * Přístupové heslo k administraci.
 * Hodnotu mezi apostrofy můžete změnit na libovolné vlastní heslo.
 */
const PROVISIONER_ADMIN_PASSWORD = 'Essensadmin21+';

session_name('provisioner_admin');

$isHttps = (
    (!empty($_SERVER['HTTPS']) && strtolower((string) $_SERVER['HTTPS']) !== 'off')
    || (int) ($_SERVER['SERVER_PORT'] ?? 0) === 443
);

session_set_cookie_params([
    'lifetime' => 0,
    'path' => '/',
    'secure' => $isHttps,
    'httponly' => true,
    'samesite' => 'Lax',
]);

session_start();

header('Cache-Control: no-store, no-cache, must-revalidate, max-age=0');
header('Pragma: no-cache');
header('X-Content-Type-Options: nosniff');
header('X-Frame-Options: SAMEORIGIN');
header('Referrer-Policy: same-origin');

function h(string $value): string
{
    return htmlspecialchars($value, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
}

function redirectToAdmin(): void
{
    $target = isset($_SERVER['SCRIPT_NAME']) && is_string($_SERVER['SCRIPT_NAME'])
        ? $_SERVER['SCRIPT_NAME']
        : 'index.php';

    $target = str_replace(["\r", "\n"], '', $target);
    header('Location: ' . ($target !== '' ? $target : 'index.php'), true, 303);
    exit;
}

function destroyAdminSession(): void
{
    $_SESSION = [];

    if ((bool) ini_get('session.use_cookies')) {
        $cookie = session_get_cookie_params();
        setcookie(
            session_name(),
            '',
            time() - 42000,
            $cookie['path'],
            $cookie['domain'],
            (bool) $cookie['secure'],
            (bool) $cookie['httponly']
        );
    }

    session_destroy();
}

$isAuthenticated = isset($_SESSION['provisioner_authenticated'])
    && $_SESSION['provisioner_authenticated'] === true;
$loginError = '';
$requestMethod = strtoupper((string) ($_SERVER['REQUEST_METHOD'] ?? 'GET'));
$formAction = isset($_POST['form_action']) && is_string($_POST['form_action'])
    ? $_POST['form_action']
    : '';

if ($requestMethod === 'POST' && $formAction === 'login' && !$isAuthenticated) {
    $submittedPassword = isset($_POST['password']) && is_string($_POST['password'])
        ? $_POST['password']
        : '';

    if (hash_equals(PROVISIONER_ADMIN_PASSWORD, $submittedPassword)) {
        session_regenerate_id(true);
        $_SESSION['provisioner_authenticated'] = true;
        $_SESSION['csrf_token'] = bin2hex(random_bytes(32));
        redirectToAdmin();
    }

    usleep(300000);
    $loginError = 'Zadané heslo není správné.';
}

if ($requestMethod === 'POST' && $formAction === 'logout') {
    if ($isAuthenticated) {
        $sessionToken = isset($_SESSION['csrf_token']) && is_string($_SESSION['csrf_token'])
            ? $_SESSION['csrf_token']
            : '';
        $submittedToken = isset($_POST['csrf_token']) && is_string($_POST['csrf_token'])
            ? $_POST['csrf_token']
            : '';

        if ($sessionToken === '' || !hash_equals($sessionToken, $submittedToken)) {
            http_response_code(403);
            exit('Bezpečnostní token pro odhlášení je neplatný. Obnovte stránku a zkuste to znovu.');
        }

        destroyAdminSession();
    }

    redirectToAdmin();
}

$isAuthenticated = isset($_SESSION['provisioner_authenticated'])
    && $_SESSION['provisioner_authenticated'] === true;

if (!$isAuthenticated) {
    ?>
<!doctype html>
<html lang="cs">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Přihlášení | Administrace Provisioneru</title>
    <link href="assets/vendor/bootstrap.min.css" rel="stylesheet">
</head>
<body class="bg-body-tertiary">
<main class="container min-vh-100 d-flex align-items-center justify-content-center py-5">
    <section class="card border-0 shadow-sm w-100" style="max-width: 430px;">
        <div class="card-body p-4 p-md-5">
            <div class="text-center mb-4">
                <h1 class="h3 mb-2">Administrace Provisioneru</h1>
                <p class="text-body-secondary mb-0">Pro pokračování zadejte přístupové heslo.</p>
            </div>

            <?php if ($loginError !== ''): ?>
                <div class="alert alert-danger" role="alert"><?= h($loginError) ?></div>
            <?php endif; ?>

            <form method="post" autocomplete="on">
                <input type="hidden" name="form_action" value="login">

                <div class="mb-3">
                    <label for="password" class="form-label">Heslo</label>
                    <input
                        type="password"
                        class="form-control form-control-lg"
                        id="password"
                        name="password"
                        required
                        autofocus
                        autocomplete="current-password"
                    >
                </div>

                <button type="submit" class="btn btn-primary btn-lg w-100">Přihlásit se</button>
            </form>
        </div>
    </section>
</main>
</body>
</html>
    <?php
    exit;
}

if (!isset($_SESSION['csrf_token']) || !is_string($_SESSION['csrf_token'])) {
    $_SESSION['csrf_token'] = bin2hex(random_bytes(32));
}

$config = require __DIR__ . DIRECTORY_SEPARATOR . 'config.php';
if (!is_array($config)) {
    throw new RuntimeException('Konfigurační soubor config.php musí vracet pole.');
}

$installerBaseUrl = isset($config['installerBaseUrl']) && is_string($config['installerBaseUrl'])
    ? $config['installerBaseUrl']
    : 'https://static.essensworld.com/helpdesk/';

$clientConfig = [
    'apiUrl' => 'api.php',
    'loginUrl' => 'index.php',
    'csrfToken' => $_SESSION['csrf_token'],
    'installerBaseUrl' => $installerBaseUrl,
];

$uploadMax = (string) ini_get('upload_max_filesize');
$postMax = (string) ini_get('post_max_size');
?>
<!doctype html>
<html lang="cs">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Administrace Provisioneru</title>
    <link href="assets/vendor/bootstrap.min.css" rel="stylesheet">
    <link href="assets/admin.css" rel="stylesheet">
</head>
<body>
<nav class="navbar navbar-expand-lg bg-dark navbar-dark shadow-sm">
    <div class="container-xxl">
        <span class="navbar-brand mb-0 h1">Administrace Provisioneru</span>
        <div class="d-flex align-items-center gap-3">
            <span class="navbar-text text-white-50">apps.json</span>
            <form method="post" class="m-0">
                <input type="hidden" name="form_action" value="logout">
                <input type="hidden" name="csrf_token" value="<?= h($_SESSION['csrf_token']) ?>">
                <button type="submit" class="btn btn-sm btn-outline-light">Odhlásit se</button>
            </form>
        </div>
    </div>
</nav>

<main class="container-xxl py-4 py-lg-5">
    <div class="d-flex flex-column flex-lg-row align-items-lg-center justify-content-between gap-3 mb-4">
        <div>
            <h1 class="h3 mb-1">Aplikace</h1>
            <p class="text-body-secondary mb-0">
                Přidávejte instalátory, upravujte jejich nastavení, měňte stav nebo je trvale odstraňujte.
            </p>
        </div>
        <div class="d-flex flex-wrap gap-2">
            <button type="button" class="btn btn-outline-secondary" id="refreshButton">
                Obnovit
            </button>
            <button type="button" class="btn btn-primary" id="createButton">
                Přidat aplikaci
            </button>
        </div>
    </div>

    <div id="pageAlert" class="alert d-none" role="alert"></div>

    <section class="card border-0 shadow-sm">
        <div class="card-header bg-body py-3">
            <div class="d-flex flex-column flex-md-row align-items-md-center justify-content-between gap-3">
                <div class="d-flex align-items-center gap-2">
                    <h2 class="h5 mb-0">Seznam instalátorů</h2>
                    <span class="badge text-bg-secondary" id="appCount">0</span>
                </div>
                <div class="search-box">
                    <label for="searchInput" class="visually-hidden">Hledat aplikace</label>
                    <input
                        type="search"
                        class="form-control"
                        id="searchInput"
                        placeholder="Hledat podle názvu, popisu, souboru nebo URL"
                        autocomplete="off"
                    >
                </div>
            </div>
        </div>

        <div class="table-responsive">
            <table class="table table-hover align-middle mb-0" id="appsTable">
                <thead class="table-light">
                <tr>
                    <th scope="col" class="name-column">Název</th>
                    <th scope="col">Popis</th>
                    <th scope="col" class="actions-column text-end">Akce</th>
                </tr>
                </thead>
                <tbody id="appsTableBody">
                <tr>
                    <td colspan="3" class="text-center py-5 text-body-secondary">
                        <div class="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></div>
                        Načítám aplikace…
                    </td>
                </tr>
                </tbody>
            </table>
        </div>
    </section>
</main>

<div class="toast-container position-fixed top-0 end-0 p-3" id="toastContainer" aria-live="polite" aria-atomic="true"></div>

<div class="modal fade" id="appModal" tabindex="-1" aria-labelledby="appModalTitle" aria-hidden="true">
    <div class="modal-dialog modal-xl modal-dialog-scrollable">
        <div class="modal-content">
            <form id="appForm" enctype="multipart/form-data" novalidate>
                <div class="modal-header">
                    <div>
                        <h2 class="modal-title fs-5" id="appModalTitle">Přidat aplikaci</h2>
                        <div class="small text-body-secondary" id="appModalSubtitle">
                            Nová aplikace dostane číselné ID a bude ve výchozím stavu zakázaná.
                        </div>
                    </div>
                    <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Zavřít"></button>
                </div>

                <div class="modal-body">
                    <div id="formError" class="alert alert-danger d-none" role="alert"></div>
                    <input type="hidden" id="appId" value="">

                    <div class="row g-4">
                        <div class="col-lg-7">
                            <div class="mb-3">
                                <label for="appName" class="form-label">Název <span class="text-danger">*</span></label>
                                <input type="text" class="form-control" id="appName" required autofocus>
                                <div class="invalid-feedback">Zadejte název aplikace.</div>
                            </div>

                            <div class="mb-3">
                                <label for="appDescription" class="form-label">Popis</label>
                                <textarea class="form-control" id="appDescription" rows="4"></textarea>
                            </div>

                            <div class="mb-3" id="installerUploadGroup">
                                <label for="installerFile" class="form-label">
                                    Instalační soubor <span class="text-danger">*</span>
                                </label>
                                <input
                                    type="file"
                                    class="form-control"
                                    id="installerFile"
                                    name="installerFile"
                                    accept=".exe,.msi"
                                    required
                                >
                                <div class="form-text upload-note">
                                    Povolené jsou pouze soubory <code>.exe</code> a <code>.msi</code>.
                                    Soubor se uloží do nadřazené složky a URL se vytvoří automaticky jako
                                    <code><?= h(rtrim($installerBaseUrl, '/') . '/<název-souboru>') ?></code>.
                                    Limity serveru: <code>upload_max_filesize=<?= h($uploadMax) ?></code>,
                                    <code>post_max_size=<?= h($postMax) ?></code>.
                                </div>
                                <div class="invalid-feedback">Vyberte instalační soubor EXE nebo MSI.</div>
                            </div>

                            <div class="row g-3">
                                <div class="col-md-5">
                                    <label for="installerType" class="form-label">
                                        Typ instalátoru <span class="text-danger">*</span>
                                    </label>
                                    <select class="form-select" id="installerType" required>
                                        <option value="auto">Automaticky</option>
                                        <option value="exe">EXE</option>
                                        <option value="msi">MSI</option>
                                    </select>
                                    <div class="invalid-feedback">Typ musí odpovídat nahranému souboru.</div>
                                </div>
                                <div class="col-md-7">
                                    <label for="fileName" class="form-label">Název souboru při stažení</label>
                                    <input
                                        type="text"
                                        class="form-control font-monospace"
                                        id="fileName"
                                        placeholder="Např. instalator.exe"
                                    >
                                    <div class="form-text">
                                        Název, pod kterým se instalátor uloží do počítače uživatele.
                                        Cesty ke složkám nejsou povoleny. Při vytvoření se prázdná hodnota doplní názvem nahraného souboru.
                                    </div>
                                </div>
                            </div>

                            <div class="mt-3">
                                <label for="sha256" class="form-label">Kontrolní součet SHA-256</label>
                                <input
                                    type="text"
                                    class="form-control font-monospace"
                                    id="sha256"
                                    maxlength="64"
                                    pattern="[A-Fa-f0-9]{64}"
                                    placeholder="Volitelný 64znakový hexadecimální kontrolní součet"
                                >
                                <div class="invalid-feedback">
                                    Zadejte přesně 64 hexadecimálních znaků, nebo pole ponechte prázdné.
                                </div>
                            </div>

                            <div class="form-check form-switch mt-4">
                                <input class="form-check-input" type="checkbox" role="switch" id="waitForInstaller" checked>
                                <label class="form-check-label" for="waitForInstaller">
                                    Počkat na dokončení procesu instalátoru
                                </label>
                            </div>
                        </div>

                        <div class="col-lg-5">
                            <section class="repeater-card mb-4" aria-labelledby="argumentsHeading">
                                <div class="d-flex justify-content-between align-items-center gap-2 mb-3">
                                    <div>
                                        <h3 class="h6 mb-1" id="argumentsHeading">Argumenty</h3>
                                        <p class="small text-body-secondary mb-0">
                                            Jeden argument příkazového řádku na každý řádek.
                                        </p>
                                    </div>
                                    <button type="button" class="btn btn-sm btn-outline-primary" id="addArgumentButton">
                                        Přidat argument
                                    </button>
                                </div>
                                <div id="argumentsList" class="d-grid gap-2"></div>
                            </section>

                            <section class="repeater-card" aria-labelledby="exitCodesHeading">
                                <div class="d-flex justify-content-between align-items-center gap-2 mb-3">
                                    <div>
                                        <h3 class="h6 mb-1" id="exitCodesHeading">Úspěšné návratové kódy</h3>
                                        <p class="small text-body-secondary mb-0">
                                            Celočíselné návratové kódy procesu považované za úspěch.
                                        </p>
                                    </div>
                                    <button type="button" class="btn btn-sm btn-outline-primary" id="addExitCodeButton">
                                        Přidat kód
                                    </button>
                                </div>
                                <div id="exitCodesList" class="d-grid gap-2"></div>
                            </section>
                        </div>
                    </div>
                </div>

                <div class="modal-footer">
                    <button type="button" class="btn btn-outline-secondary" data-bs-dismiss="modal">Zrušit</button>
                    <button type="submit" class="btn btn-primary" id="saveButton">
                        <span class="button-label">Přidat aplikaci</span>
                        <span class="spinner-border spinner-border-sm ms-2 d-none" role="status" aria-hidden="true"></span>
                    </button>
                </div>
            </form>
        </div>
    </div>
</div>

<div class="modal fade" id="deleteModal" tabindex="-1" aria-labelledby="deleteModalTitle" aria-hidden="true">
    <div class="modal-dialog modal-dialog-centered">
        <div class="modal-content">
            <div class="modal-header">
                <h2 class="modal-title fs-5" id="deleteModalTitle">Smazat aplikaci</h2>
                <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Zavřít"></button>
            </div>
            <div class="modal-body">
                <p class="mb-2">Opravdu smazat aplikaci <strong id="deleteAppName"></strong>?</p>
                <p class="text-body-secondary mb-2">
                    Záznam bude trvale odstraněn z <code>apps.json</code> a současně se smaže jeho nahraný
                    instalační soubor EXE/MSI z nadřazené složky.
                </p>
                <p class="small text-warning-emphasis mb-0">
                    Pouhé zakázání aplikace instalační soubor nemaže.
                </p>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-outline-secondary" data-bs-dismiss="modal">Zrušit</button>
                <button type="button" class="btn btn-danger" id="confirmDeleteButton">
                    <span class="button-label">Smazat</span>
                    <span class="spinner-border spinner-border-sm ms-2 d-none" role="status" aria-hidden="true"></span>
                </button>
            </div>
        </div>
    </div>
</div>

<script>
window.ProvisionerAdminConfig = <?= json_encode(
    $clientConfig,
    JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT
) ?>;
</script>
<script src="assets/vendor/jquery-3.7.1.min.js"></script>
<script src="assets/vendor/bootstrap.bundle.min.js"></script>
<script src="assets/admin.js"></script>
</body>
</html>
