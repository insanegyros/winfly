<?php

declare(strict_types=1);

final class ProvisionerJsonCRUD
{
    private const INSTALLER_TYPES = ['exe', 'msi', 'auto'];

    private const MUTABLE_FIELDS = [
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

    private string $filePath;
    private string $lockPath;

    public function __construct(?string $filePath = null)
    {
        $this->filePath = $filePath ?? __DIR__ . DIRECTORY_SEPARATOR . 'apps.json';
        $this->lockPath = $this->filePath . '.lock';

        $directory = dirname($this->filePath);
        if (!is_dir($directory)) {
            throw new RuntimeException("Složka neexistuje: {$directory}");
        }

        $this->withExclusiveLock(function (): void {
            if (!is_file($this->filePath)) {
                $this->writeDocumentUnlocked(['apps' => []]);
            }
        });

        // Check that the file contains valid JSON and the expected root structure.
        $this->withSharedLock(function (): void {
            $this->readDocumentUnlocked();
        });
    }

    /**
     * @return array<int, array<string, mixed>>
     */
    public function getAll(): array
    {
        return $this->withSharedLock(function (): array {
            $document = $this->readDocumentUnlocked();
            $this->assertValidIds($document['apps']);

            return array_values($document['apps']);
        });
    }

    /**
     * @return array<string, mixed>|null
     */
    public function getById(int $id): ?array
    {
        $this->assertPositiveId($id);

        return $this->withSharedLock(function () use ($id): ?array {
            $document = $this->readDocumentUnlocked();
            $this->assertValidIds($document['apps']);

            foreach ($document['apps'] as $app) {
                if ($app['id'] === $id) {
                    return $app;
                }
            }

            return null;
        });
    }

    /**
     * Adds a disabled app with an automatically generated numeric ID.
     *
     * @param array<string, mixed> $data
     * @return array<string, mixed>
     */
    public function create(array $data): array
    {
        $this->assertAllowedInputFields($data);

        return $this->withExclusiveLock(function () use ($data): array {
            $document = $this->readDocumentUnlocked();
            $this->assertValidIds($document['apps']);

            $id = $this->getNextId($document['apps']);
            $app = $this->normalizeApp($id, false, $data);

            $document['apps'][] = $app;
            $this->writeDocumentUnlocked($document);

            return $app;
        });
    }

    /**
     * Partially updates mutable fields. The id, enabled, and url fields are rejected.
     *
     * @param array<string, mixed> $changes
     * @return array<string, mixed>
     */
    public function update(int $id, array $changes): array
    {
        $this->assertPositiveId($id);

        if (array_key_exists('url', $changes)) {
            throw new InvalidArgumentException(
                'Pole url nelze po vytvoření aplikace změnit. URL vzniká automaticky z nahraného souboru.'
            );
        }

        $this->assertAllowedInputFields($changes);

        return $this->withExclusiveLock(function () use ($id, $changes): array {
            $document = $this->readDocumentUnlocked();
            $this->assertValidIds($document['apps']);
            $index = $this->requireAppIndex($document['apps'], $id);
            $current = $document['apps'][$index];

            $mutableCurrent = [];
            foreach (self::MUTABLE_FIELDS as $field) {
                if (array_key_exists($field, $current)) {
                    $mutableCurrent[$field] = $current[$field];
                }
            }

            $updated = $this->normalizeApp(
                $id,
                $this->normalizeStoredEnabled($current, $id),
                array_merge($mutableCurrent, $changes)
            );

            $document['apps'][$index] = $updated;
            $this->writeDocumentUnlocked($document);

            return $updated;
        });
    }

    /**
     * Removes an app and returns the removed record.
     *
     * @return array<string, mixed>
     */
    public function delete(int $id): array
    {
        $this->assertPositiveId($id);

        return $this->withExclusiveLock(function () use ($id): array {
            $document = $this->readDocumentUnlocked();
            $this->assertValidIds($document['apps']);
            $index = $this->requireAppIndex($document['apps'], $id);

            $removed = $document['apps'][$index];
            array_splice($document['apps'], $index, 1);
            $document['apps'] = array_values($document['apps']);

            $this->writeDocumentUnlocked($document);

            return $removed;
        });
    }

    /**
     * @return array<string, mixed>
     */
    public function enable(int $id): array
    {
        return $this->setEnabled($id, true);
    }

    /**
     * @return array<string, mixed>
     */
    public function disable(int $id): array
    {
        return $this->setEnabled($id, false);
    }

    /**
     * @return array<string, mixed>
     */
    public function setEnabled(int $id, bool $enabled): array
    {
        $this->assertPositiveId($id);

        return $this->withExclusiveLock(function () use ($id, $enabled): array {
            $document = $this->readDocumentUnlocked();
            $this->assertValidIds($document['apps']);
            $index = $this->requireAppIndex($document['apps'], $id);

            $document['apps'][$index]['enabled'] = $enabled;
            $updated = $document['apps'][$index];

            $this->writeDocumentUnlocked($document);

            return $updated;
        });
    }

    /**
     * Converts old string/missing/duplicate IDs to unique positive integers.
     * Valid existing integer IDs are preserved whenever possible.
     *
     * @return array<int, array{oldId: mixed, newId: int}>
     */
    public function migrateIdsToIntegers(): array
    {
        return $this->withExclusiveLock(function (): array {
            $document = $this->readDocumentUnlocked();
            $apps = $document['apps'];
            $reserved = [];
            $candidates = [];

            foreach ($apps as $index => $app) {
                if (!is_array($app)) {
                    throw new RuntimeException("Aplikace na indexu {$index} musí být objektem JSON.");
                }

                $candidate = $this->positiveIntegerCandidate($app['id'] ?? null);
                if ($candidate !== null && !isset($reserved[$candidate])) {
                    $reserved[$candidate] = true;
                    $candidates[$index] = $candidate;
                } else {
                    $candidates[$index] = null;
                }
            }

            $nextId = $reserved === [] ? 1 : max(array_keys($reserved)) + 1;
            $mapping = [];
            $changed = false;

            foreach ($apps as $index => &$app) {
                $oldId = $app['id'] ?? null;
                $newId = $candidates[$index];

                if ($newId === null) {
                    while (isset($reserved[$nextId])) {
                        ++$nextId;
                    }
                    $newId = $nextId;
                    $reserved[$newId] = true;
                    ++$nextId;
                }

                if (!is_int($oldId) || $oldId !== $newId) {
                    $mapping[] = [
                        'oldId' => $oldId,
                        'newId' => $newId,
                    ];
                    $changed = true;
                }

                $app['id'] = $newId;
            }
            unset($app);

            if ($changed) {
                $document['apps'] = array_values($apps);
                $this->writeDocumentUnlocked($document);
            }

            return $mapping;
        });
    }

    /**
     * @param array<string, mixed> $data
     * @return array<string, mixed>
     */
    private function normalizeApp(int $id, bool $enabled, array $data): array
    {
        $defaults = [
            'description' => '',
            'sha256' => '',
            'installerType' => 'auto',
            'arguments' => [],
            'wait' => true,
            'successExitCodes' => [0, 1641, 3010],
        ];

        $data = array_merge($defaults, $data);

        $name = $this->requireString($data, 'name', false);
        $description = $this->requireString($data, 'description', true);
        $url = $this->normalizeUrl($this->requireString($data, 'url', false));
        $sha256 = strtolower($this->requireString($data, 'sha256', true));
        $installerType = strtolower($this->requireString($data, 'installerType', false));

        if (!in_array($installerType, self::INSTALLER_TYPES, true)) {
            throw new InvalidArgumentException(
                'installerType musí být jedna z hodnot: ' . implode(', ', self::INSTALLER_TYPES)
            );
        }

        if ($sha256 !== '' && preg_match('/^[a-f0-9]{64}$/D', $sha256) !== 1) {
            throw new InvalidArgumentException('sha256 musí být prázdné nebo obsahovat přesně 64 hexadecimálních znaků.');
        }

        $fileName = '';
        if (array_key_exists('fileName', $data)) {
            if (!is_string($data['fileName'])) {
                throw new InvalidArgumentException('fileName musí být textový řetězec.');
            }
            $fileName = trim($data['fileName']);
        }

        if ($fileName === '') {
            $fileName = $this->generateFileName($id, $installerType, $url);
        }
        $this->assertSafeFileName($fileName);

        return [
            'id' => $id,
            'name' => $name,
            'description' => $description,
            'enabled' => $enabled,
            'url' => $url,
            'sha256' => $sha256,
            'fileName' => $fileName,
            'installerType' => $installerType,
            'arguments' => $this->normalizeStringArray($data['arguments'], 'arguments'),
            'wait' => $this->normalizeBoolean($data['wait'], 'wait'),
            'successExitCodes' => $this->normalizeIntegerArray(
                $data['successExitCodes'],
                'successExitCodes'
            ),
        ];
    }

    /**
     * @param array<string, mixed> $data
     */
    private function assertAllowedInputFields(array $data): void
    {
        $allowed = array_fill_keys(self::MUTABLE_FIELDS, true);

        foreach (array_keys($data) as $field) {
            if ($field === 'id' || $field === 'enabled') {
                throw new InvalidArgumentException(
                    "Pole '{$field}' zde nelze změnit. Použijte vygenerované ID nebo metodu enable()/disable()."
                );
            }

            if (!isset($allowed[$field])) {
                throw new InvalidArgumentException("Neznámé pole aplikace: {$field}");
            }
        }
    }

    /**
     * @param array<string, mixed> $data
     */
    private function requireString(array $data, string $field, bool $allowEmpty): string
    {
        if (!array_key_exists($field, $data)) {
            throw new InvalidArgumentException("Chybí povinné pole: {$field}");
        }

        if (!is_string($data[$field])) {
            throw new InvalidArgumentException("{$field} musí být textový řetězec.");
        }

        $value = trim($data[$field]);
        if (!$allowEmpty && $value === '') {
            throw new InvalidArgumentException("{$field} nesmí být prázdné.");
        }

        return $value;
    }

    private function normalizeUrl(string $url): string
    {
        if (filter_var($url, FILTER_VALIDATE_URL) === false) {
            throw new InvalidArgumentException('url musí být platná URL.');
        }

        $scheme = strtolower((string) parse_url($url, PHP_URL_SCHEME));
        if ($scheme !== 'http' && $scheme !== 'https') {
            throw new InvalidArgumentException('url musí používat HTTP nebo HTTPS.');
        }

        return $url;
    }

    /**
     * @return array<int, string>
     */
    private function normalizeStringArray($value, string $field): array
    {
        if (!is_array($value)) {
            throw new InvalidArgumentException("{$field} musí být pole textových řetězců.");
        }

        $result = [];
        foreach ($value as $index => $item) {
            if (!is_string($item)) {
                throw new InvalidArgumentException("{$field}[{$index}] musí být textový řetězec.");
            }
            $result[] = trim($item);
        }

        return $result;
    }

    /**
     * @return array<int, int>
     */
    private function normalizeIntegerArray($value, string $field): array
    {
        if (!is_array($value)) {
            throw new InvalidArgumentException("{$field} musí být pole celých čísel.");
        }

        $result = [];
        $seen = [];

        foreach ($value as $index => $item) {
            if (is_int($item)) {
                $integer = $item;
            } elseif (is_string($item) && preg_match('/^-?[0-9]+$/D', $item) === 1) {
                $validated = filter_var($item, FILTER_VALIDATE_INT, FILTER_NULL_ON_FAILURE);
                if ($validated === null) {
                    throw new InvalidArgumentException("{$field}[{$index}] je mimo rozsah celého čísla.");
                }
                $integer = $validated;
            } else {
                throw new InvalidArgumentException("{$field}[{$index}] musí být celé číslo.");
            }

            $key = (string) $integer;
            if (!isset($seen[$key])) {
                $seen[$key] = true;
                $result[] = $integer;
            }
        }

        return $result;
    }

    private function normalizeBoolean($value, string $field): bool
    {
        if (!is_bool($value)) {
            throw new InvalidArgumentException("{$field} musí být logická hodnota true/false.");
        }

        return $value;
    }

    private function generateFileName(int $id, string $installerType, string $url): string
    {
        if ($installerType === 'exe' || $installerType === 'msi') {
            $extension = $installerType;
        } else {
            $path = (string) parse_url($url, PHP_URL_PATH);
            $urlExtension = strtolower((string) pathinfo($path, PATHINFO_EXTENSION));
            $extension = in_array($urlExtension, ['exe', 'msi'], true) ? $urlExtension : 'bin';
        }

        return "installer-{$id}.{$extension}";
    }

    private function assertSafeFileName(string $fileName): void
    {
        if ($fileName === '' || $fileName === '.' || $fileName === '..') {
            throw new InvalidArgumentException('fileName není platný název souboru.');
        }

        if (strpos($fileName, "\0") !== false || preg_match('/[\\\\\/]/', $fileName) === 1) {
            throw new InvalidArgumentException('fileName nesmí obsahovat cestu ke složce.');
        }
    }

    /**
     * @return array{apps: array<int, array<string, mixed>>}&array<string, mixed>
     */
    private function readDocumentUnlocked(): array
    {
        $json = @file_get_contents($this->filePath);
        if ($json === false) {
            throw new RuntimeException("Soubor {$this->filePath} se nepodařilo načíst.");
        }

        if (trim($json) === '') {
            throw new RuntimeException("Soubor {$this->filePath} je prázdný.");
        }

        try {
            $document = json_decode($json, true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $exception) {
            throw new RuntimeException(
                "Neplatný JSON v souboru {$this->filePath}: {$exception->getMessage()}",
                0,
                $exception
            );
        }

        if (!is_array($document) || !array_key_exists('apps', $document) || !is_array($document['apps'])) {
            throw new RuntimeException("Soubor {$this->filePath} musí obsahovat kořenový objekt s polem apps.");
        }

        foreach ($document['apps'] as $index => $app) {
            if (!is_array($app)) {
                throw new RuntimeException("Aplikace na indexu {$index} musí být objektem JSON.");
            }
        }

        return $document;
    }

    /**
     * @param array<string, mixed> $document
     */
    private function writeDocumentUnlocked(array $document): void
    {
        try {
            $json = json_encode(
                $document,
                JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR
            );
        } catch (JsonException $exception) {
            throw new RuntimeException('Data aplikací se nepodařilo převést do JSON: ' . $exception->getMessage(), 0, $exception);
        }

        $json .= PHP_EOL;
        $directory = dirname($this->filePath);
        $temporaryPath = tempnam($directory, '.apps-json-');

        if ($temporaryPath === false) {
            throw new RuntimeException("Ve složce {$directory} se nepodařilo vytvořit dočasný soubor.");
        }

        try {
            $handle = @fopen($temporaryPath, 'wb');
            if ($handle === false) {
                throw new RuntimeException("Dočasný soubor {$temporaryPath} se nepodařilo otevřít.");
            }

            try {
                $length = strlen($json);
                $written = 0;

                while ($written < $length) {
                    $chunk = fwrite($handle, substr($json, $written));
                    if ($chunk === false || $chunk === 0) {
                        throw new RuntimeException("Do dočasného souboru {$temporaryPath} se nepodařilo zapisovat.");
                    }
                    $written += $chunk;
                }

                if (!fflush($handle)) {
                    throw new RuntimeException("Dočasný soubor {$temporaryPath} se nepodařilo zapsat na disk.");
                }

                if (function_exists('fsync')) {
                    @fsync($handle);
                }
            } finally {
                fclose($handle);
            }

            $permissions = is_file($this->filePath)
                ? ((int) fileperms($this->filePath) & 0777)
                : 0664;
            @chmod($temporaryPath, $permissions);

            if (!@rename($temporaryPath, $this->filePath)) {
                throw new RuntimeException("Soubor {$this->filePath} se nepodařilo nahradit novou verzí.");
            }
        } finally {
            if (is_file($temporaryPath)) {
                @unlink($temporaryPath);
            }
        }
    }

    /**
     * @param array<int, array<string, mixed>> $apps
     */
    private function assertValidIds(array $apps): void
    {
        $seen = [];

        foreach ($apps as $index => $app) {
            $id = $app['id'] ?? null;
            if (!is_int($id) || $id < 1) {
                throw new RuntimeException(
                    "Aplikace na indexu {$index} nemá kladné celočíselné ID. " .
                    'Pro starší textová ID jednou spusťte migrateIdsToIntegers().'
                );
            }

            if (isset($seen[$id])) {
                throw new RuntimeException(
                    "Duplicitní ID aplikace {$id}. Pro opravu jednou spusťte migrateIdsToIntegers()."
                );
            }

            $seen[$id] = true;
        }
    }

    /**
     * @param array<int, array<string, mixed>> $apps
     */
    private function getNextId(array $apps): int
    {
        $maxId = 0;
        foreach ($apps as $app) {
            $maxId = max($maxId, $app['id']);
        }

        if ($maxId === PHP_INT_MAX) {
            throw new RuntimeException('Nejsou k dispozici žádná další číselná ID aplikací.');
        }

        return $maxId + 1;
    }

    /**
     * @param array<int, array<string, mixed>> $apps
     */
    private function requireAppIndex(array $apps, int $id): int
    {
        foreach ($apps as $index => $app) {
            if ($app['id'] === $id) {
                return $index;
            }
        }

        throw new OutOfBoundsException("Aplikace s ID {$id} nebyla nalezena.");
    }

    /**
     * @param array<string, mixed> $app
     */
    private function normalizeStoredEnabled(array $app, int $id): bool
    {
        if (!array_key_exists('enabled', $app) || !is_bool($app['enabled'])) {
            throw new RuntimeException("Aplikace s ID {$id} má neplatnou hodnotu enabled.");
        }

        return $app['enabled'];
    }

    private function assertPositiveId(int $id): void
    {
        if ($id < 1) {
            throw new InvalidArgumentException('ID aplikace musí být kladné celé číslo.');
        }
    }

    private function positiveIntegerCandidate($value): ?int
    {
        if (is_int($value)) {
            return $value > 0 ? $value : null;
        }

        if (!is_string($value) || preg_match('/^[1-9][0-9]*$/D', $value) !== 1) {
            return null;
        }

        $validated = filter_var($value, FILTER_VALIDATE_INT, FILTER_NULL_ON_FAILURE);
        return is_int($validated) && $validated > 0 ? $validated : null;
    }

    private function withSharedLock(callable $callback)
    {
        return $this->withLock(LOCK_SH, $callback);
    }

    private function withExclusiveLock(callable $callback)
    {
        return $this->withLock(LOCK_EX, $callback);
    }

    private function withLock(int $mode, callable $callback)
    {
        $handle = @fopen($this->lockPath, 'c');
        if ($handle === false) {
            throw new RuntimeException("Zamykací soubor {$this->lockPath} se nepodařilo otevřít.");
        }

        try {
            if (!flock($handle, $mode)) {
                throw new RuntimeException("Soubor {$this->filePath} se nepodařilo zamknout.");
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
}
