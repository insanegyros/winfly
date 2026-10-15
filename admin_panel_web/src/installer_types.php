<?php

declare(strict_types=1);

// Installer types and file name rules. They mirror the WinFly client
// (internal/config/installer_type.go and IsSafeInstallerFileName in
// internal/config/config.go), so the panel never saves a list the client rejects.

/** File types that can be uploaded and installed. */
const INSTALLER_EXTENSIONS = ['exe', 'msi', 'iso', 'img'];

/** Values of installerType; "auto" means "decide by the fileName extension". */
const INSTALLER_TYPES = ['auto', 'exe', 'msi', 'iso', 'img'];

/** MSI and disk images must be waited for (the client rejects the list otherwise). */
const INSTALLER_TYPES_REQUIRING_WAIT = ['msi', 'iso', 'img'];

const WINDOWS_RESERVED_NAMES = [
    'CON', 'PRN', 'AUX', 'NUL',
    'COM1', 'COM2', 'COM3', 'COM4', 'COM5', 'COM6', 'COM7', 'COM8', 'COM9',
    'LPT1', 'LPT2', 'LPT3', 'LPT4', 'LPT5', 'LPT6', 'LPT7', 'LPT8', 'LPT9',
];

/**
 * Returns exe, msi, iso or img by the file extension (any letter case), or null.
 */
function installerTypeFromFileName(string $fileName): ?string
{
    $extension = strtolower(pathinfo($fileName, PATHINFO_EXTENSION));

    return in_array($extension, INSTALLER_EXTENSIONS, true) ? $extension : null;
}

/**
 * Returns the installer type the client will use, or null when "auto" cannot
 * tell it from the file name.
 */
function resolveInstallerType(string $installerType, string $fileName): ?string
{
    return $installerType === 'auto' ? installerTypeFromFileName($fileName) : $installerType;
}

function installerTypeRequiresWait(?string $resolvedType): bool
{
    return in_array($resolvedType, INSTALLER_TYPES_REQUIRING_WAIT, true);
}

/**
 * Rejects anything that is not a plain, safe Windows file name.
 */
function assertSafeFileName(string $fileName, string $label): void
{
    $stem = strtoupper(explode('.', $fileName)[0]);

    if ($fileName === '' || $fileName === '.' || $fileName === '..' || rtrim($fileName, ' .') !== $fileName) {
        throw new ApiError("{$label} není platný název souboru (nesmí končit tečkou ani mezerou).");
    }

    if (preg_match('/[\\\\\/:*?"<>|\x00-\x1F\x7F]/', $fileName) === 1) {
        throw new ApiError("{$label} nesmí obsahovat cestu ani znaky \\ / : * ? \" < > |.");
    }

    if (strlen($fileName) > 240 || in_array($stem, WINDOWS_RESERVED_NAMES, true)) {
        throw new ApiError("{$label} je příliš dlouhý nebo jde o vyhrazený název systému Windows.");
    }
}
