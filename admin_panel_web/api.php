<?php

declare(strict_types=1);

// JSON API used by assets/js. The action is chosen by ?action=, see ApiActions::ROUTES.

require __DIR__ . '/src/bootstrap.php';

startAdminSession();
sendSecurityHeaders();
header('Content-Type: application/json; charset=utf-8');

if (!isLoggedIn()) {
    sendJson(['success' => false, 'message' => 'Přihlášení vypršelo nebo nemáte oprávnění. Přihlaste se znovu.'], 401);
}

$isCsrfTokenValid = isValidCsrfToken($_SERVER['HTTP_X_CSRF_TOKEN'] ?? null);
$sessionId = session_id();
// Release the session lock so parallel chunk uploads do not wait for each other.
session_write_close();

try {
    $action = strtolower(trim((string) ($_GET['action'] ?? 'list')));
    if (!isset(ApiActions::ROUTES[$action])) {
        throw ApiError::notFound('Neznámá akce API.');
    }

    [$method, $handler] = ApiActions::ROUTES[$action];
    requireRequestMethod($method);
    if ($method === 'POST' && !$isCsrfTokenValid) {
        throw new ApiError('Bezpečnostní token je neplatný nebo vypršel. Obnovte stránku a akci zopakujte.', 403, 'INVALID_CSRF_TOKEN');
    }

    $actions = new ApiActions(loadSettings(), $sessionId);
    sendJson(['success' => true] + $actions->$handler());
} catch (ApiError $error) {
    sendJson(['success' => false, 'message' => $error->getMessage(), 'code' => $error->errorCode()], $error->httpStatus());
} catch (Throwable $exception) {
    error_log((string) $exception);
    sendJson(['success' => false, 'message' => $exception->getMessage()], 500);
}
