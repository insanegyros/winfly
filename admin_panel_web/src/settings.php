<?php

declare(strict_types=1);

/**
 * Loads config.php and returns every setting validated, with defaults filled in.
 *
 * @return array{
 *     adminPassword: string,
 *     appsFile: string,
 *     installerDirectory: string,
 *     installerBaseUrl: string,
 *     chunkUploadDirectory: string,
 *     chunkSizeBytes: int,
 *     chunkMinimumSizeBytes: int,
 *     chunkUploadMaxAgeSeconds: int
 * }
 */
function loadSettings(): array
{
    $adminDirectory = dirname(__DIR__);
    $config = require $adminDirectory . '/config.php';
    if (!is_array($config)) {
        throw new RuntimeException('Konfigurační soubor config.php musí vracet pole.');
    }

    $chunkSize = settingInteger($config, 'chunkSizeBytes', 1048576, 16384, 16777216);

    return [
        'adminPassword' => settingText($config, 'adminPassword'),
        'appsFile' => settingText($config, 'appsFile'),
        'installerDirectory' => settingText($config, 'installerDirectory'),
        'installerBaseUrl' => rtrim(settingHttpUrl($config, 'installerBaseUrl'), '/') . '/',
        'chunkUploadDirectory' => settingText($config, 'chunkUploadDirectory', $adminDirectory . '/.winfly-uploads'),
        'chunkSizeBytes' => $chunkSize,
        'chunkMinimumSizeBytes' => settingInteger($config, 'chunkMinimumSizeBytes', 65536, 16384, $chunkSize),
        'chunkUploadMaxAgeSeconds' => settingInteger($config, 'chunkUploadMaxAgeSeconds', 86400, 300, 604800),
    ];
}

/**
 * @param array<string, mixed> $config
 */
function settingText(array $config, string $key, ?string $default = null): string
{
    $value = $config[$key] ?? $default;
    if (!is_string($value) || trim($value) === '') {
        throw new RuntimeException("V config.php chybí platná hodnota {$key}.");
    }

    return trim($value);
}

/**
 * @param array<string, mixed> $config
 */
function settingHttpUrl(array $config, string $key): string
{
    $url = settingText($config, $key);
    if (!isHttpUrl($url)) {
        throw new RuntimeException("Hodnota {$key} v config.php musí být platná URL s HTTP nebo HTTPS.");
    }

    return $url;
}

/**
 * @param array<string, mixed> $config
 */
function settingInteger(array $config, string $key, int $default, int $minimum, int $maximum): int
{
    $value = $config[$key] ?? $default;
    if (is_string($value) && ctype_digit($value)) {
        $value = (int) $value;
    }

    if (!is_int($value) || $value < $minimum || $value > $maximum) {
        throw new RuntimeException("Hodnota {$key} v config.php musí být celé číslo od {$minimum} do {$maximum}.");
    }

    return $value;
}

function isHttpUrl(string $url): bool
{
    $scheme = strtolower((string) parse_url($url, PHP_URL_SCHEME));

    return filter_var($url, FILTER_VALIDATE_URL) !== false && ($scheme === 'http' || $scheme === 'https');
}
