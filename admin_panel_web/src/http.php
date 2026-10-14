<?php

declare(strict_types=1);

// Request and response helpers of api.php.

/**
 * Sends $payload as JSON and ends the request.
 *
 * @param array<string, mixed> $payload
 */
function sendJson(array $payload, int $status = 200): void
{
    try {
        $json = encodeJson($payload);
    } catch (JsonException $exception) {
        $status = 500;
        $json = '{"success":false,"message":"Odpověď API se nepodařilo převést do JSON."}';
    }

    http_response_code($status);
    echo $json;
    exit;
}

function requireRequestMethod(string $method): void
{
    if (strtoupper((string) ($_SERVER['REQUEST_METHOD'] ?? 'GET')) !== $method) {
        header('Allow: ' . $method);
        throw new ApiError("Tato akce vyžaduje požadavek {$method}.", 405, 'METHOD_NOT_ALLOWED');
    }
}

/**
 * @return array<string, mixed>
 */
function readJsonBody(): array
{
    $body = (string) file_get_contents('php://input');
    if (trim($body) === '') {
        return [];
    }

    try {
        return decodeJsonObject($body, 'Tělo požadavku');
    } catch (RuntimeException $exception) {
        throw new ApiError($exception->getMessage());
    }
}

/**
 * Accepts an integer or a string of digits that is at least $minimum.
 *
 * @param mixed $value
 */
function requireInteger($value, string $label, int $minimum): int
{
    if (is_string($value) && preg_match('/^\d+$/D', $value) === 1) {
        $value = filter_var($value, FILTER_VALIDATE_INT, FILTER_NULL_ON_FAILURE);
    }

    if (!is_int($value) || $value < $minimum) {
        throw new ApiError("{$label} musí být celé číslo, nejméně {$minimum}.");
    }

    return $value;
}

/**
 * @param array<string, mixed> $data
 */
function requireNonEmptyText(array $data, string $key, string $label): string
{
    $value = $data[$key] ?? null;
    if (!is_string($value) || trim($value) === '') {
        throw new ApiError("{$label} je povinná hodnota.");
    }

    return trim($value);
}
