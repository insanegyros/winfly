<?php defined('WINFLY_ADMIN') || exit; ?>
<?php $pageTitle = 'Přihlášení | Administrace WinFly'; require __DIR__ . '/head.php'; ?>
<body class="bg-body-tertiary">
<main class="container min-vh-100 d-flex align-items-center justify-content-center py-5">
    <section class="card border-0 shadow-sm w-100 login-card">
        <div class="card-body p-4 p-md-5">
            <div class="text-center mb-4">
                <h1 class="h3 mb-2">Administrace WinFly</h1>
                <p class="text-body-secondary mb-0">Pro pokračování zadejte přístupové heslo.</p>
            </div>

            <?php if ($errorMessage !== ''): ?>
                <div class="alert alert-danger" role="alert"><?= h($errorMessage) ?></div>
            <?php endif; ?>

            <form method="post" autocomplete="on">
                <input type="hidden" name="form_action" value="login">
                <div class="mb-3">
                    <label for="password" class="form-label">Heslo</label>
                    <input type="password" class="form-control form-control-lg" id="password" name="password"
                           required autofocus autocomplete="current-password">
                </div>
                <button type="submit" class="btn btn-primary btn-lg w-100">Přihlásit se</button>
            </form>
        </div>
    </section>
</main>
</body>
</html>
