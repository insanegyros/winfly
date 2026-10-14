<?php

declare(strict_types=1);

// Login state of the administrator, kept in the PHP session.

const SESSION_LOGGED_IN = 'winfly_logged_in';
const SESSION_CSRF_TOKEN = 'winfly_csrf_token';

function startAdminSession(): void
{
    session_name('winfly_admin');
    session_set_cookie_params([
        'lifetime' => 0,
        'path' => '/',
        'secure' => isHttpsRequest(),
        'httponly' => true,
        'samesite' => 'Lax',
    ]);
    session_start();
}

function sendSecurityHeaders(): void
{
    header('Cache-Control: no-store, no-cache, must-revalidate, max-age=0');
    header('Pragma: no-cache');
    header('X-Content-Type-Options: nosniff');
    header('X-Frame-Options: SAMEORIGIN');
    header('Referrer-Policy: same-origin');
}

function isHttpsRequest(): bool
{
    $https = strtolower((string) ($_SERVER['HTTPS'] ?? ''));

    return ($https !== '' && $https !== 'off') || (int) ($_SERVER['SERVER_PORT'] ?? 0) === 443;
}

function isLoggedIn(): bool
{
    return ($_SESSION[SESSION_LOGGED_IN] ?? false) === true;
}

/**
 * Logs in when the password is right. A wrong password is answered slowly to
 * make guessing harder.
 */
function logIn(string $password, string $adminPassword): bool
{
    if (!hash_equals($adminPassword, $password)) {
        usleep(300000);
        return false;
    }

    session_regenerate_id(true);
    $_SESSION[SESSION_LOGGED_IN] = true;
    $_SESSION[SESSION_CSRF_TOKEN] = bin2hex(random_bytes(32));

    return true;
}

function logOut(): void
{
    $_SESSION = [];

    if (ini_get('session.use_cookies')) {
        $cookie = session_get_cookie_params();
        setcookie(session_name(), '', time() - 42000, $cookie['path'], $cookie['domain'], $cookie['secure'], $cookie['httponly']);
    }

    session_destroy();
}

function csrfToken(): string
{
    if (!is_string($_SESSION[SESSION_CSRF_TOKEN] ?? null)) {
        $_SESSION[SESSION_CSRF_TOKEN] = bin2hex(random_bytes(32));
    }

    return $_SESSION[SESSION_CSRF_TOKEN];
}

/**
 * @param mixed $submittedToken
 */
function isValidCsrfToken($submittedToken): bool
{
    return is_string($submittedToken) && hash_equals(csrfToken(), $submittedToken);
}
