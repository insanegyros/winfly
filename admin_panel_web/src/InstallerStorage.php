<?php

declare(strict_types=1);

/**
 * The public folder with uploaded installers (installerDirectory) and its
 * public URL (installerBaseUrl).
 */
final class InstallerStorage
{
    private string $directory;
    private string $baseUrl;

    public function __construct(string $directory, string $baseUrl)
    {
        $this->directory = rtrim($directory, '/\\');
        $this->baseUrl = $baseUrl;
    }

    public function directory(): string
    {
        return $this->directory;
    }

    public function pathOf(string $fileName): string
    {
        return $this->directory . '/' . $fileName;
    }

    public function urlOf(string $fileName): string
    {
        return $this->baseUrl . rawurlencode($fileName);
    }

    /**
     * Returns the installer file name when $url points directly into the
     * managed folder, otherwise null.
     */
    public function managedFileNameOf(string $url): ?string
    {
        $prefix = $this->comparableUrl($this->baseUrl);
        $target = $this->comparableUrl($url);
        if ($prefix === null || $target === null || strncmp($target, $prefix, strlen($prefix)) !== 0) {
            return null;
        }

        $fileName = substr($target, strlen($prefix));
        if (preg_match('/[\/\\\\\x00]/', $fileName) === 1 || installerTypeFromFileName($fileName) === null) {
            return null;
        }

        return $fileName;
    }

    /**
     * @param array<int, array<string, mixed>> $apps
     */
    public function isUsedByAnyOf(string $fileName, array $apps): bool
    {
        foreach ($apps as $app) {
            if (is_string($app['url'] ?? null) && $this->managedFileNameOf($app['url']) === $fileName) {
                return true;
            }
        }

        return false;
    }

    /**
     * Renames the installer to a hidden temporary name, so it can be restored
     * when removing the app record fails. Returns the temporary path.
     */
    public function moveToQuarantine(string $fileName): string
    {
        $quarantinePath = $this->directory . '/.winfly-delete-' . bin2hex(random_bytes(12));
        if (is_dir($this->pathOf($fileName)) || !@rename($this->pathOf($fileName), $quarantinePath)) {
            throw new RuntimeException("Instalační soubor {$fileName} se nepodařilo připravit ke smazání. Zkontrolujte oprávnění nadřazené složky.");
        }

        return $quarantinePath;
    }

    public function restoreFromQuarantine(string $quarantinePath, string $fileName): bool
    {
        return !pathExists($this->pathOf($fileName)) && @rename($quarantinePath, $this->pathOf($fileName));
    }

    /**
     * Lower-cased scheme and host, port and decoded path, so two URLs of the
     * same file compare equal.
     */
    private function comparableUrl(string $url): ?string
    {
        $parts = parse_url($url);
        if (!is_array($parts) || !isset($parts['scheme'], $parts['host'])) {
            return null;
        }

        return strtolower($parts['scheme'] . '://' . $parts['host'])
            . (isset($parts['port']) ? ':' . $parts['port'] : '')
            . rawurldecode($parts['path'] ?? '/');
    }
}
