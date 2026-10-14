<?php defined('WINFLY_ADMIN') || exit; ?>
<?php
$pageTitle = 'Administrace WinFly';
$browserConfig = [
    'apiUrl' => 'api.php',
    'loginUrl' => 'index.php',
    'csrfToken' => csrfToken(),
];
require __DIR__ . '/head.php';
?>
<body>
<nav class="navbar navbar-expand-lg bg-dark navbar-dark shadow-sm">
    <div class="container-xxl">
        <span class="navbar-brand mb-0 h1">Administrace WinFly</span>
        <div class="d-flex align-items-center gap-3">
            <span class="navbar-text text-white-50">apps.json</span>
            <form method="post" class="m-0">
                <input type="hidden" name="form_action" value="logout">
                <input type="hidden" name="csrf_token" value="<?= h(csrfToken()) ?>">
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
            <button type="button" class="btn btn-outline-secondary" id="refreshButton">Obnovit</button>
            <button type="button" class="btn btn-primary" data-action="create">Přidat aplikaci</button>
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
                    <input type="search" class="form-control" id="searchInput" autocomplete="off"
                           placeholder="Hledat podle názvu, popisu, souboru nebo URL">
                </div>
            </div>
        </div>

        <div class="table-responsive">
            <table class="table table-hover align-middle mb-0">
                <thead class="table-light">
                <tr>
                    <th scope="col" class="name-column">Název</th>
                    <th scope="col">Popis</th>
                    <th scope="col" class="actions-column text-end">Akce</th>
                </tr>
                </thead>
                <tbody id="appsTableBody"></tbody>
            </table>
        </div>
    </section>
</main>

<div class="toast-container position-fixed top-0 end-0 p-3" id="toastContainer" aria-live="polite" aria-atomic="true"></div>

<?php require __DIR__ . '/app_modal.php'; ?>
<?php require __DIR__ . '/delete_modal.php'; ?>

<script>
window.WinFlyConfig = <?= json_encode($browserConfig, JSON_UNESCAPED_SLASHES | JSON_HEX_TAG | JSON_HEX_AMP | JSON_HEX_APOS | JSON_HEX_QUOT) ?>;
</script>
<script src="assets/vendor/jquery-3.7.1.min.js"></script>
<script src="assets/vendor/bootstrap.bundle.min.js"></script>
<script src="assets/js/helpers.js"></script>
<script src="assets/js/api.js"></script>
<script src="assets/js/ui.js"></script>
<script src="assets/js/chunk-upload.js"></script>
<script src="assets/js/app-table.js"></script>
<script src="assets/js/app-form.js"></script>
<script src="assets/js/main.js"></script>
</body>
</html>
