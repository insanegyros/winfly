<?php

declare(strict_types=1);

// Validation of one app record in apps.json. The order of keys in the returned
// record is the order the client and earlier panel versions use.

/** Fields the administrator may set; id and enabled are managed by the panel. */
const APP_EDITABLE_FIELDS = [
    'name',
    'description',
    'url',
    'sha256',
    'fileName',
    'installerType',
    'arguments',
    'wait',
    'successExitCodes',
];

const APP_DEFAULTS = [
    'description' => '',
    'sha256' => '',
    'fileName' => '',
    'installerType' => 'auto',
    'arguments' => [],
    'wait' => true,
    'successExitCodes' => [0, 1641, 3010],
];

/**
 * @param array<string, mixed> $data
 */
function assertOnlyEditableFields(array $data): void
{
    foreach (array_keys($data) as $field) {
        if (!in_array($field, APP_EDITABLE_FIELDS, true)) {
            throw new ApiError("Pole aplikace '{$field}' nelze tímto způsobem nastavit.");
        }
    }
}

/**
 * @param array<string, mixed> $app
 * @return array<string, mixed>
 */
function editableFieldsOf(array $app): array
{
    return array_intersect_key($app, array_flip(APP_EDITABLE_FIELDS));
}

/**
 * Validates $data and returns the complete record as it is stored in apps.json.
 *
 * @param array<string, mixed> $data
 * @return array<string, mixed>
 */
function buildAppRecord(int $id, bool $enabled, array $data): array
{
    $data = array_merge(APP_DEFAULTS, $data);

    $name = requireTextField($data, 'name', false);
    $url = requireTextField($data, 'url', false);
    if (!isHttpUrl($url)) {
        throw new ApiError('url musí být platná URL s HTTP nebo HTTPS.');
    }

    $sha256 = strtolower(requireTextField($data, 'sha256', true));
    if ($sha256 !== '' && preg_match('/^[a-f0-9]{64}$/D', $sha256) !== 1) {
        throw new ApiError('sha256 musí být prázdné nebo obsahovat přesně 64 hexadecimálních znaků.');
    }

    $installerType = strtolower(requireTextField($data, 'installerType', false));
    if (!in_array($installerType, INSTALLER_TYPES, true)) {
        throw new ApiError('installerType musí být jedna z hodnot: ' . implode(', ', INSTALLER_TYPES) . '.');
    }

    $fileName = requireTextField($data, 'fileName', true);
    if ($fileName === '') {
        $fileName = defaultDownloadFileName($id, $installerType, $url);
    }
    assertSafeFileName($fileName, 'fileName');

    $wait = $data['wait'];
    if (!is_bool($wait)) {
        throw new ApiError('wait musí být logická hodnota true/false.');
    }

    $resolvedType = resolveInstallerType($installerType, $fileName);
    if (!$wait && installerTypeRequiresWait($resolvedType)) {
        throw new ApiError('U instalátoru ' . strtoupper((string) $resolvedType) . ' musí být zapnuto čekání na dokončení.');
    }

    return [
        'id' => $id,
        'name' => $name,
        'description' => requireTextField($data, 'description', true),
        'enabled' => $enabled,
        'url' => $url,
        'sha256' => $sha256,
        'fileName' => $fileName,
        'installerType' => $installerType,
        'arguments' => requireTextList($data['arguments'], 'arguments'),
        'wait' => $wait,
        'successExitCodes' => requireIntegerList($data['successExitCodes'], 'successExitCodes'),
    ];
}

/**
 * Throws when the installer type would not match the real file, for example
 * type "msi" for an uploaded .exe, or "auto" with a fileName ending in .exe
 * for an uploaded .iso.
 *
 * @param array<string, mixed> $data
 */
function assertInstallerTypeMatchesFile(array $data, string $fileExtension, string $defaultFileName): void
{
    $installerType = strtolower(trim((string) ($data['installerType'] ?? 'auto')));
    $fileName = trim((string) ($data['fileName'] ?? ''));
    $resolvedType = resolveInstallerType($installerType, $fileName !== '' ? $fileName : $defaultFileName);

    if ($resolvedType === $fileExtension) {
        return;
    }

    if ($installerType === 'auto') {
        throw new ApiError("Název souboru při stažení musí končit příponou .{$fileExtension}, jinak WinFly nepozná typ instalátoru.");
    }

    throw new ApiError("Zvolený typ instalátoru {$installerType} neodpovídá souboru .{$fileExtension}.");
}

function defaultDownloadFileName(int $id, string $installerType, string $url): string
{
    $extension = $installerType !== 'auto'
        ? $installerType
        : installerTypeFromFileName(rawurldecode((string) parse_url($url, PHP_URL_PATH))) ?? 'bin';

    return "installer-{$id}.{$extension}";
}

/**
 * @param array<string, mixed> $data
 */
function requireTextField(array $data, string $field, bool $allowEmpty): string
{
    $value = $data[$field] ?? null;
    if (!is_string($value)) {
        throw new ApiError("{$field} musí být textový řetězec.");
    }

    $value = trim($value);
    if (!$allowEmpty && $value === '') {
        throw new ApiError("{$field} nesmí být prázdné.");
    }

    return $value;
}

/**
 * @param mixed $value
 * @return array<int, string>
 */
function requireTextList($value, string $field): array
{
    if (!is_array($value)) {
        throw new ApiError("{$field} musí být pole textových řetězců.");
    }

    $result = [];
    foreach ($value as $index => $item) {
        if (!is_string($item)) {
            throw new ApiError("{$field}[{$index}] musí být textový řetězec.");
        }
        $result[] = trim($item);
    }

    return $result;
}

/**
 * Returns the integers without duplicates; numeric strings are accepted.
 *
 * @param mixed $value
 * @return array<int, int>
 */
function requireIntegerList($value, string $field): array
{
    if (!is_array($value)) {
        throw new ApiError("{$field} musí být pole celých čísel.");
    }

    $result = [];
    foreach ($value as $index => $item) {
        $integer = filter_var($item, FILTER_VALIDATE_INT, FILTER_NULL_ON_FAILURE);
        if (is_bool($item) || is_float($item) || !is_int($integer)) {
            throw new ApiError("{$field}[{$index}] musí být celé číslo.");
        }
        if (!in_array($integer, $result, true)) {
            $result[] = $integer;
        }
    }

    return $result;
}
