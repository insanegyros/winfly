<?php

declare(strict_types=1);

// Small file helpers shared by the app list and the chunked upload:
// JSON encoding, locking, atomic writes and directory removal.

/**
 * @param mixed $data
 */
function encodeJson($data, bool $pretty = false): string
{
    $flags = JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR;

    return json_encode($data, $pretty ? $flags | JSON_PRETTY_PRINT : $flags);
}

/**
 * Decodes a JSON object or array; anything else is an error.
 *
 * @return array<mixed>
 */
function decodeJsonObject(string $json, string $source): array
{
    try {
        $data = json_decode($json, true, 512, JSON_THROW_ON_ERROR);
    } catch (JsonException $exception) {
        throw new RuntimeException("{$source} neobsahuje platný JSON: {$exception->getMessage()}", 0, $exception);
    }

    if (!is_array($data)) {
        throw new RuntimeException("{$source} musí obsahovat objekt JSON.");
    }

    return $data;
}

/**
 * @return array<mixed>
 */
function readJsonFile(string $path): array
{
    $json = @file_get_contents($path);
    if ($json === false || trim($json) === '') {
        throw new RuntimeException("Soubor {$path} se nepodařilo načíst nebo je prázdný.");
    }

    return decodeJsonObject($json, "Soubor {$path}");
}

/**
 * Writes pretty-printed JSON through a temporary file and a rename, so readers
 * never see a half-written file.
 *
 * @param array<mixed> $data
 */
function writeJsonFile(string $path, array $data): void
{
    $directory = dirname($path);
    $temporaryPath = tempnam($directory, '.tmp-');
    if ($temporaryPath === false) {
        throw new RuntimeException("Ve složce {$directory} se nepodařilo vytvořit dočasný soubor.");
    }

    try {
        $handle = @fopen($temporaryPath, 'wb');
        if ($handle === false) {
            throw new RuntimeException("Dočasný soubor {$temporaryPath} se nepodařilo otevřít.");
        }

        try {
            writeAllBytes($handle, encodeJson($data, true) . PHP_EOL);
            if (function_exists('fsync')) {
                @fsync($handle);
            }
        } finally {
            fclose($handle);
        }

        @chmod($temporaryPath, is_file($path) ? fileperms($path) & 0777 : 0664);

        if (!@rename($temporaryPath, $path)) {
            throw new RuntimeException("Soubor {$path} se nepodařilo nahradit novou verzí.");
        }
    } finally {
        if (is_file($temporaryPath)) {
            @unlink($temporaryPath);
        }
    }
}

/**
 * @param resource $handle
 */
function writeAllBytes($handle, string $data): void
{
    $length = strlen($data);

    for ($offset = 0; $offset < $length; $offset += $written) {
        $written = fwrite($handle, substr($data, $offset));
        if ($written === false || $written === 0) {
            throw new RuntimeException('Data se nepodařilo zapsat na disk.');
        }
    }

    if (!fflush($handle)) {
        throw new RuntimeException('Data se nepodařilo zapsat na disk.');
    }
}

/**
 * Runs $callback while holding a lock on $lockPath and returns its result.
 * $mode is LOCK_SH or LOCK_EX, optionally with LOCK_NB.
 *
 * @return mixed
 */
function withFileLock(string $lockPath, int $mode, callable $callback)
{
    $handle = @fopen($lockPath, 'c');
    if ($handle === false) {
        throw new RuntimeException("Zamykací soubor {$lockPath} se nepodařilo otevřít.");
    }

    try {
        if (!flock($handle, $mode)) {
            throw new RuntimeException("Soubor {$lockPath} se nepodařilo zamknout.");
        }

        try {
            return $callback();
        } finally {
            flock($handle, LOCK_UN);
        }
    } finally {
        fclose($handle);
    }
}

function pathExists(string $path): bool
{
    return file_exists($path) || is_link($path);
}

function createDirectory(string $path): void
{
    if (!is_dir($path) && !@mkdir($path, 0700, true) && !is_dir($path)) {
        throw new RuntimeException("Složku {$path} se nepodařilo vytvořit.");
    }
}

function removeDirectory(string $directory): void
{
    if (is_link($directory) || is_file($directory)) {
        @unlink($directory);
        return;
    }

    if (!is_dir($directory)) {
        return;
    }

    foreach (scandir($directory) ?: [] as $item) {
        if ($item !== '.' && $item !== '..') {
            removeDirectory($directory . '/' . $item);
        }
    }

    @rmdir($directory);
}
