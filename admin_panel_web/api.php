<?php

declare(strict_types=1);

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

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store, no-cache, must-revalidate, max-age=0');
header('X-Content-Type-Options: nosniff');
header('X-Frame-Options: SAMEORIGIN');
header('Referrer-Policy: same-origin');

if (
    !isset($_SESSION['provisioner_authenticated'])
    || $_SESSION['provisioner_authenticated'] !== true
) {
    http_response_code(401);
    echo json_encode([
        'success' => false,
        'message' => 'Přihlášení vypršelo nebo nemáte oprávnění. Přihlaste se znovu.',
    ], JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);
    exit;
}

require_once __DIR__ . DIRECTORY_SEPARATOR . 'ProvisionerJsonCRUD.php';
require_once __DIR__ . DIRECTORY_SEPARATOR . 'ProvisionerChunkUpload.php';

/**
 * @param array<string, mixed> $payload
 */
function respond(array $payload, int $status = 200): void
{
    http_response_code($status);

    try {
        echo json_encode(
            $payload,
            JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR
        );
    } catch (JsonException $exception) {
        http_response_code(500);
        echo '{"success":false,"message":"Odpověď API se nepodařilo převést do JSON."}';
    }

    exit;
}

function requireMethod(string $expected): void
{
    $actual = strtoupper((string) ($_SERVER['REQUEST_METHOD'] ?? 'GET'));
    if ($actual !== $expected) {
        header('Allow: ' . $expected);
        respond([
            'success' => false,
            'message' => "Tato akce vyžaduje požadavek {$expected}.",
        ], 405);
    }
}

/**
 * @return array<string, mixed>
 */
function readJsonPayload(): array
{
    $rawBody = file_get_contents('php://input');
    if ($rawBody === false) {
        throw new RuntimeException('Tělo požadavku se nepodařilo načíst.');
    }

    if (trim($rawBody) === '') {
        return [];
    }

    try {
        $payload = json_decode($rawBody, true, 512, JSON_THROW_ON_ERROR);
    } catch (JsonException $exception) {
        throw new InvalidArgumentException('Tělo požadavku musí obsahovat platný JSON.', 0, $exception);
    }

    if (!is_array($payload)) {
        throw new InvalidArgumentException('Tělo požadavku musí obsahovat objekt JSON.');
    }

    return $payload;
}

/**
 * @param mixed $value
 */
function requirePositiveIntegerValue($value, string $label): int
{
    if (is_int($value) && $value > 0) {
        return $value;
    }

    if (is_string($value) && preg_match('/^[1-9][0-9]*$/D', $value) === 1) {
        $validated = filter_var($value, FILTER_VALIDATE_INT, FILTER_NULL_ON_FAILURE);
        if (is_int($validated) && $validated > 0) {
            return $validated;
        }
    }

    throw new InvalidArgumentException("{$label} musí být kladné celé číslo.");
}

/**
 * @param mixed $value
 */
function requireNonNegativeIntegerValue($value, string $label): int
{
    if (is_int($value) && $value >= 0) {
        return $value;
    }

    if (is_string($value) && preg_match('/^(0|[1-9][0-9]*)$/D', $value) === 1) {
        $validated = filter_var($value, FILTER_VALIDATE_INT, FILTER_NULL_ON_FAILURE);
        if (is_int($validated) && $validated >= 0) {
            return $validated;
        }
    }

    throw new InvalidArgumentException("{$label} musí být nezáporné celé číslo.");
}

/**
 * @param array<string, mixed> $payload
 */
function requireId(array $payload): int
{
    return requirePositiveIntegerValue($payload['id'] ?? null, 'ID aplikace');
}

/**
 * @param array<string, mixed> $payload
 * @return array<string, mixed>
 */
function requireAppData(array $payload): array
{
    $app = $payload['app'] ?? null;
    if (!is_array($app)) {
        throw new InvalidArgumentException('Požadavek musí obsahovat objekt app.');
    }

    return $app;
}

/**
 * @param array<string, mixed> $payload
 */
function requirePayloadString(array $payload, string $key, string $label): string
{
    $value = $payload[$key] ?? null;
    if (!is_string($value) || trim($value) === '') {
        throw new InvalidArgumentException("{$label} je povinná hodnota.");
    }

    return trim($value);
}

function verifyCsrfToken(string $sessionToken): void
{
    $requestToken = $_SERVER['HTTP_X_CSRF_TOKEN'] ?? '';

    if (
        $sessionToken === ''
        || !is_string($requestToken)
        || !hash_equals($sessionToken, $requestToken)
    ) {
        respond([
            'success' => false,
            'message' => 'Bezpečnostní token je neplatný nebo vypršel. Obnovte stránku a akci zopakujte.',
        ], 403);
    }
}

function normalizeInstallerBaseUrl(string $baseUrl): string
{
    $baseUrl = trim($baseUrl);
    if (filter_var($baseUrl, FILTER_VALIDATE_URL) === false) {
        throw new RuntimeException('installerBaseUrl v config.php není platná URL.');
    }

    $scheme = strtolower((string) parse_url($baseUrl, PHP_URL_SCHEME));
    if ($scheme !== 'http' && $scheme !== 'https') {
        throw new RuntimeException('installerBaseUrl v config.php musí používat HTTP nebo HTTPS.');
    }

    return rtrim($baseUrl, '/') . '/';
}

/**
 * @param array<string, mixed> $config
 */
function requireConfiguredString(array $config, string $key): string
{
    $value = $config[$key] ?? null;
    if (!is_string($value) || trim($value) === '') {
        throw new RuntimeException("V config.php chybí platná hodnota {$key}.");
    }

    return trim($value);
}

/**
 * @param array<string, mixed> $config
 */
function optionalConfiguredString(array $config, string $key, string $default): string
{
    $value = $config[$key] ?? $default;
    if (!is_string($value) || trim($value) === '') {
        throw new RuntimeException("Hodnota {$key} v config.php musí být neprázdný text.");
    }

    return trim($value);
}

/**
 * @param array<string, mixed> $config
 */
function optionalConfiguredInteger(
    array $config,
    string $key,
    int $default,
    int $minimum,
    int $maximum
): int {
    $value = $config[$key] ?? $default;

    if (is_string($value) && preg_match('/^[0-9]+$/D', $value) === 1) {
        $value = filter_var($value, FILTER_VALIDATE_INT, FILTER_NULL_ON_FAILURE);
    }

    if (!is_int($value) || $value < $minimum || $value > $maximum) {
        throw new RuntimeException(
            "Hodnota {$key} v config.php musí být celé číslo od {$minimum} do {$maximum}."
        );
    }

    return $value;
}

function installerUrl(string $baseUrl, string $fileName): string
{
    return $baseUrl . rawurlencode($fileName);
}

/**
 * Vrátí fyzický název souboru pouze tehdy, když URL míří přímo do spravované veřejné složky.
 */
function managedInstallerFileName(string $url, string $baseUrl): ?string
{
    $urlParts = parse_url($url);
    $baseParts = parse_url($baseUrl);

    if (!is_array($urlParts) || !is_array($baseParts)) {
        return null;
    }

    if (
        strtolower((string) ($urlParts['scheme'] ?? '')) !== strtolower((string) ($baseParts['scheme'] ?? ''))
        || strtolower((string) ($urlParts['host'] ?? '')) !== strtolower((string) ($baseParts['host'] ?? ''))
        || (string) ($urlParts['port'] ?? '') !== (string) ($baseParts['port'] ?? '')
    ) {
        return null;
    }

    $basePath = rawurldecode((string) ($baseParts['path'] ?? '/'));
    $urlPath = rawurldecode((string) ($urlParts['path'] ?? ''));

    if ($basePath === '' || substr($basePath, -1) !== '/') {
        $basePath .= '/';
    }

    if (strncmp($urlPath, $basePath, strlen($basePath)) !== 0) {
        return null;
    }

    $fileName = substr($urlPath, strlen($basePath));
    if (
        $fileName === ''
        || $fileName === '.'
        || $fileName === '..'
        || strpos($fileName, "\0") !== false
        || preg_match('/[\/\\\\]/', $fileName) === 1
    ) {
        return null;
    }

    $extension = strtolower((string) pathinfo($fileName, PATHINFO_EXTENSION));
    if (!in_array($extension, ['exe', 'msi'], true)) {
        return null;
    }

    return $fileName;
}

function installerExtensionFromUrl(string $url, string $baseUrl): ?string
{
    $managedName = managedInstallerFileName($url, $baseUrl);
    if ($managedName !== null) {
        return strtolower((string) pathinfo($managedName, PATHINFO_EXTENSION));
    }

    $path = parse_url($url, PHP_URL_PATH);
    if (!is_string($path)) {
        return null;
    }

    $extension = strtolower((string) pathinfo(rawurldecode($path), PATHINFO_EXTENSION));
    return in_array($extension, ['exe', 'msi'], true) ? $extension : null;
}

/**
 * @param array<string, mixed> $appData
 */
function assertInstallerTypeMatches(array $appData, string $extension): void
{
    if (!array_key_exists('installerType', $appData) || !is_string($appData['installerType'])) {
        return;
    }

    $installerType = strtolower(trim($appData['installerType']));
    if ($installerType !== 'auto' && $installerType !== $extension) {
        throw new InvalidArgumentException(
            "Zvolený typ instalátoru {$installerType} neodpovídá nahranému souboru .{$extension}."
        );
    }
}

/**
 * @param array<int, array<string, mixed>> $apps
 */
function isInstallerReferencedByAnotherApp(
    array $apps,
    int $deletedId,
    string $fileName,
    string $baseUrl
): bool {
    foreach ($apps as $app) {
        if (!is_array($app) || (int) ($app['id'] ?? 0) === $deletedId) {
            continue;
        }

        $otherUrl = $app['url'] ?? null;
        if (!is_string($otherUrl)) {
            continue;
        }

        if (managedInstallerFileName($otherUrl, $baseUrl) === $fileName) {
            return true;
        }
    }

    return false;
}

function createQuarantinePath(string $installerDirectory): string
{
    for ($attempt = 0; $attempt < 10; ++$attempt) {
        $candidate = $installerDirectory
            . DIRECTORY_SEPARATOR
            . '.provisioner-delete-'
            . bin2hex(random_bytes(12));

        if (!file_exists($candidate) && !is_link($candidate)) {
            return $candidate;
        }
    }

    throw new RuntimeException('Nepodařilo se vytvořit dočasný název pro bezpečné smazání instalátoru.');
}

$sessionToken = isset($_SESSION['csrf_token']) && is_string($_SESSION['csrf_token'])
    ? $_SESSION['csrf_token']
    : '';
$sessionOwnerToken = session_id();
session_write_close();

$action = strtolower(trim((string) ($_GET['action'] ?? 'list')));

try {
    $config = require __DIR__ . DIRECTORY_SEPARATOR . 'config.php';
    if (!is_array($config)) {
        throw new RuntimeException('Konfigurační soubor config.php musí vracet pole.');
    }

    $appsFile = requireConfiguredString($config, 'appsFile');
    $installerDirectory = requireConfiguredString($config, 'installerDirectory');
    $installerBaseUrl = normalizeInstallerBaseUrl(
        requireConfiguredString($config, 'installerBaseUrl')
    );

    $repository = new ProvisionerJsonCRUD($appsFile);
    $chunkUpload = null;

    if (in_array($action, ['upload-init', 'upload-chunk', 'upload-complete', 'upload-cancel'], true)) {
        $chunkUploadDirectory = optionalConfiguredString(
            $config,
            'chunkUploadDirectory',
            __DIR__ . DIRECTORY_SEPARATOR . '.provisioner-uploads'
        );
        $chunkSizeBytes = optionalConfiguredInteger(
            $config,
            'chunkSizeBytes',
            1048576,
            16384,
            16777216
        );
        $chunkMinimumSizeBytes = optionalConfiguredInteger(
            $config,
            'chunkMinimumSizeBytes',
            65536,
            16384,
            $chunkSizeBytes
        );
        $chunkUploadMaxAgeSeconds = optionalConfiguredInteger(
            $config,
            'chunkUploadMaxAgeSeconds',
            86400,
            300,
            604800
        );

        $chunkUpload = new ProvisionerChunkUpload(
            $chunkUploadDirectory,
            $installerDirectory,
            $sessionOwnerToken,
            $chunkSizeBytes,
            $chunkMinimumSizeBytes,
            $chunkUploadMaxAgeSeconds
        );
    }

    switch ($action) {
        case 'list':
            requireMethod('GET');
            respond([
                'success' => true,
                'apps' => $repository->getAll(),
            ]);

        case 'get':
            requireMethod('GET');
            $app = $repository->getById(requireId(['id' => $_GET['id'] ?? null]));

            if ($app === null) {
                throw new OutOfBoundsException('Požadovaná aplikace nebyla nalezena.');
            }

            respond([
                'success' => true,
                'app' => $app,
            ]);

        case 'upload-init':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);
            $payload = readJsonPayload();
            $fileName = requirePayloadString($payload, 'fileName', 'Název instalačního souboru');
            $fileSize = requirePositiveIntegerValue($payload['fileSize'] ?? null, 'Velikost instalačního souboru');
            $requestedChunkSize = null;

            if (array_key_exists('chunkSizeBytes', $payload) && $payload['chunkSizeBytes'] !== null) {
                $requestedChunkSize = requirePositiveIntegerValue(
                    $payload['chunkSizeBytes'],
                    'Velikost nahrávacího bloku'
                );
            }

            if (!($chunkUpload instanceof ProvisionerChunkUpload)) {
                throw new RuntimeException('Správce blokového nahrávání není dostupný.');
            }

            $chunkUpload->cleanupExpired();
            $upload = $chunkUpload->initialize($fileName, $fileSize, $requestedChunkSize);

            respond([
                'success' => true,
                'message' => 'Blokové nahrávání bylo připraveno.',
                'upload' => $upload,
            ], 201);

        case 'upload-chunk':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);

            $uploadIdValue = $_GET['uploadId'] ?? null;
            $chunkIndexValue = $_GET['chunkIndex'] ?? null;
            if (!is_string($uploadIdValue)) {
                throw new InvalidArgumentException('V požadavku chybí platné uploadId.');
            }

            $chunkIndex = requireNonNegativeIntegerValue($chunkIndexValue, 'Pořadové číslo bloku');
            $contentLength = null;
            if (isset($_SERVER['CONTENT_LENGTH']) && (string) $_SERVER['CONTENT_LENGTH'] !== '') {
                $contentLength = requireNonNegativeIntegerValue(
                    (string) $_SERVER['CONTENT_LENGTH'],
                    'Velikost těla požadavku'
                );
            }

            $inputStream = @fopen('php://input', 'rb');
            if ($inputStream === false) {
                throw new RuntimeException('Datový proud nahrávaného bloku se nepodařilo otevřít.');
            }

            try {
                if (!($chunkUpload instanceof ProvisionerChunkUpload)) {
                    throw new RuntimeException('Správce blokového nahrávání není dostupný.');
                }

                $chunkResult = $chunkUpload->writeChunk(
                    $uploadIdValue,
                    $chunkIndex,
                    $inputStream,
                    $contentLength
                );
            } finally {
                fclose($inputStream);
            }

            respond([
                'success' => true,
                'chunk' => $chunkResult,
            ]);

        case 'upload-complete':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);
            $payload = readJsonPayload();
            $uploadId = requirePayloadString($payload, 'uploadId', 'ID nahrávání');
            $appData = requireAppData($payload);

            if (!($chunkUpload instanceof ProvisionerChunkUpload)) {
                throw new RuntimeException('Správce blokového nahrávání není dostupný.');
            }

            $app = $chunkUpload->complete(
                $uploadId,
                function (array $storedInstaller) use (
                    $appData,
                    $repository,
                    $installerBaseUrl
                ): array {
                    assertInstallerTypeMatches($appData, $storedInstaller['extension']);
                    $data = $appData;
                    $data['url'] = installerUrl($installerBaseUrl, $storedInstaller['name']);

                    if (
                        !array_key_exists('fileName', $data)
                        || (is_string($data['fileName']) && trim($data['fileName']) === '')
                    ) {
                        $data['fileName'] = $storedInstaller['name'];
                    }

                    return $repository->create($data);
                }
            );

            respond([
                'success' => true,
                'message' => 'Aplikace byla vytvořena a instalační soubor byl nahrán po blocích.',
                'app' => $app,
            ], 201);

        case 'upload-cancel':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);
            $payload = readJsonPayload();
            $uploadId = requirePayloadString($payload, 'uploadId', 'ID nahrávání');
            if (!($chunkUpload instanceof ProvisionerChunkUpload)) {
                throw new RuntimeException('Správce blokového nahrávání není dostupný.');
            }

            $cancelled = $chunkUpload->cancel($uploadId);

            respond([
                'success' => true,
                'message' => $cancelled
                    ? 'Nahrávání bylo zrušeno a dočasná data byla odstraněna.'
                    : 'Nahrávací relace už neexistovala.',
            ]);

        case 'create':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);
            respond([
                'success' => false,
                'message' => 'Jednorázové nahrávání bylo nahrazeno blokovým přenosem. Obnovte stránku administrace.',
                'code' => 'CHUNK_UPLOAD_REQUIRED',
            ], 410);

        case 'update':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);

            $payload = readJsonPayload();
            $id = requireId($payload);
            $appData = requireAppData($payload);

            if (array_key_exists('url', $appData)) {
                throw new InvalidArgumentException(
                    'URL instalačního souboru nelze po vytvoření aplikace změnit.'
                );
            }

            $currentApp = $repository->getById($id);
            if ($currentApp === null) {
                throw new OutOfBoundsException("Aplikace s ID {$id} nebyla nalezena.");
            }

            if (isset($currentApp['url']) && is_string($currentApp['url'])) {
                $currentExtension = installerExtensionFromUrl(
                    $currentApp['url'],
                    $installerBaseUrl
                );

                if ($currentExtension !== null) {
                    assertInstallerTypeMatches($appData, $currentExtension);
                }
            }

            $app = $repository->update($id, $appData);
            respond([
                'success' => true,
                'message' => 'Změny aplikace byly uloženy.',
                'app' => $app,
            ]);

        case 'delete':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);

            $payload = readJsonPayload();
            $id = requireId($payload);
            $currentApp = $repository->getById($id);

            if ($currentApp === null) {
                throw new OutOfBoundsException("Aplikace s ID {$id} nebyla nalezena.");
            }

            $managedFileName = isset($currentApp['url']) && is_string($currentApp['url'])
                ? managedInstallerFileName($currentApp['url'], $installerBaseUrl)
                : null;

            $fileState = 'unmanaged';
            $originalInstallerPath = null;
            $quarantinePath = null;

            if ($managedFileName !== null) {
                $allApps = $repository->getAll();

                if (isInstallerReferencedByAnotherApp($allApps, $id, $managedFileName, $installerBaseUrl)) {
                    $fileState = 'shared';
                } else {
                    $originalInstallerPath = $installerDirectory . DIRECTORY_SEPARATOR . $managedFileName;

                    if (!file_exists($originalInstallerPath) && !is_link($originalInstallerPath)) {
                        $fileState = 'missing';
                    } else {
                        if (is_dir($originalInstallerPath) && !is_link($originalInstallerPath)) {
                            throw new RuntimeException(
                                "Cesta instalátoru není soubor: {$originalInstallerPath}"
                            );
                        }

                        $quarantinePath = createQuarantinePath($installerDirectory);
                        if (!@rename($originalInstallerPath, $quarantinePath)) {
                            throw new RuntimeException(
                                "Instalační soubor {$managedFileName} se nepodařilo připravit ke smazání. " .
                                'Zkontrolujte oprávnění nadřazené složky.'
                            );
                        }

                        $fileState = 'staged';
                    }
                }
            }

            try {
                $app = $repository->delete($id);
            } catch (Throwable $exception) {
                if (
                    $fileState === 'staged'
                    && is_string($quarantinePath)
                    && is_string($originalInstallerPath)
                    && (file_exists($quarantinePath) || is_link($quarantinePath))
                ) {
                    @rename($quarantinePath, $originalInstallerPath);
                }

                throw $exception;
            }

            $warning = false;

            if ($fileState === 'staged' && is_string($quarantinePath)) {
                if (@unlink($quarantinePath)) {
                    $message = 'Aplikace i její instalační soubor byly smazány.';
                } else {
                    $warning = true;
                    $restored = false;

                    if (
                        is_string($originalInstallerPath)
                        && !file_exists($originalInstallerPath)
                        && !is_link($originalInstallerPath)
                    ) {
                        $restored = @rename($quarantinePath, $originalInstallerPath);
                    }

                    $message = $restored
                        ? 'Aplikace byla smazána, ale instalační soubor se nepodařilo odstranit a byl vrácen na původní místo.'
                        : 'Aplikace byla smazána, ale dočasný instalační soubor se nepodařilo odstranit. Zkontrolujte nadřazenou složku.';
                }
            } elseif ($fileState === 'missing') {
                $message = 'Aplikace byla smazána. Instalační soubor už v nadřazené složce neexistoval.';
            } elseif ($fileState === 'shared') {
                $message = 'Aplikace byla smazána. Instalační soubor zůstal zachován, protože jej používá jiný záznam.';
            } else {
                $message = 'Aplikace byla smazána. Její URL neodkazovala na soubor spravovaný touto administrací.';
            }

            respond([
                'success' => true,
                'warning' => $warning,
                'message' => $message,
                'app' => $app,
            ]);

        case 'enable':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);
            $payload = readJsonPayload();
            $app = $repository->enable(requireId($payload));
            respond([
                'success' => true,
                'message' => 'Aplikace byla povolena.',
                'app' => $app,
            ]);

        case 'disable':
            requireMethod('POST');
            verifyCsrfToken($sessionToken);
            $payload = readJsonPayload();
            $app = $repository->disable(requireId($payload));
            respond([
                'success' => true,
                'message' => 'Aplikace byla zakázána.',
                'app' => $app,
            ]);

        default:
            respond([
                'success' => false,
                'message' => 'Neznámá akce API.',
            ], 404);
    }
} catch (ProvisionerChunkUploadException $exception) {
    respond([
        'success' => false,
        'message' => $exception->getMessage(),
        'code' => $exception->getUploadCode(),
    ], $exception->getHttpStatus());
} catch (OutOfBoundsException $exception) {
    respond([
        'success' => false,
        'message' => $exception->getMessage(),
    ], 404);
} catch (InvalidArgumentException $exception) {
    respond([
        'success' => false,
        'message' => $exception->getMessage(),
    ], 422);
} catch (Throwable $exception) {
    error_log((string) $exception);

    respond([
        'success' => false,
        'message' => $exception->getMessage(),
    ], 500);
}
