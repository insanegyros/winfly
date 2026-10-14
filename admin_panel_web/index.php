<?php

declare(strict_types=1);

// Login page and, once logged in, the app list administration.

require __DIR__ . '/src/bootstrap.php';

startAdminSession();
sendSecurityHeaders();

/**
 * Sends the browser back to this page with GET, so a reload does not repeat the form.
 */
function redirectToSelf(): void
{
    $target = str_replace(["\r", "\n"], '', (string) ($_SERVER['SCRIPT_NAME'] ?? ''));
    header('Location: ' . ($target !== '' ? $target : 'index.php'), true, 303);
    exit;
}

function postedText(string $key): string
{
    return is_string($_POST[$key] ?? null) ? $_POST[$key] : '';
}

function h(string $value): string
{
    return htmlspecialchars($value, ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8');
}

$errorMessage = '';
try {
    $settings = loadSettings();
} catch (Throwable $exception) {
    $settings = null;
    $errorMessage = $exception->getMessage();
}

$isPost = strtoupper((string) ($_SERVER['REQUEST_METHOD'] ?? 'GET')) === 'POST';
$formAction = postedText('form_action');

if ($isPost && $formAction === 'login' && !isLoggedIn() && $settings !== null) {
    if (logIn(postedText('password'), $settings['adminPassword'])) {
        redirectToSelf();
    }
    $errorMessage = 'Zadané heslo není správné.';
}

if ($isPost && $formAction === 'logout') {
    if (isLoggedIn() && !isValidCsrfToken(postedText('csrf_token'))) {
        http_response_code(403);
        exit('Bezpečnostní token pro odhlášení je neplatný. Obnovte stránku a zkuste to znovu.');
    }
    logOut();
    redirectToSelf();
}

if (!isLoggedIn() || $settings === null) {
    require __DIR__ . '/views/login.php';
    exit;
}

require __DIR__ . '/views/admin.php';
