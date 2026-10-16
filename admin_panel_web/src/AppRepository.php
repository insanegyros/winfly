<?php

declare(strict_types=1);

/**
 * The app list in apps.json. Every read takes a shared lock and every change an
 * exclusive one, and changes are written atomically, so the WinFly client can
 * download apps.json at any time.
 */
final class AppRepository
{
    private string $filePath;
    private string $lockPath;

    public function __construct(string $filePath)
    {
        if (!is_dir(dirname($filePath))) {
            throw new RuntimeException('Složka pro apps.json neexistuje: ' . dirname($filePath));
        }

        $this->filePath = $filePath;
        $this->lockPath = $filePath . '.lock';

        withFileLock($this->lockPath, LOCK_EX, function (): void {
            if (!is_file($this->filePath)) {
                writeJsonFile($this->filePath, ['apps' => []]);
            }
        });
    }

    /**
     * @return array<int, array<string, mixed>>
     */
    public function all(): array
    {
        return withFileLock($this->lockPath, LOCK_SH, function (): array {
            return $this->readDocument()['apps'];
        });
    }

    /**
     * @return array<string, mixed>
     */
    public function get(int $id): array
    {
        $apps = $this->all();

        return $apps[self::indexOf($apps, $id)];
    }

    /**
     * Adds a new, disabled app with the next free numeric ID.
     *
     * @param array<string, mixed> $data
     * @return array<string, mixed>
     */
    public function create(array $data): array
    {
        return $this->change(function (array &$apps) use ($data): array {
            $nextId = max(array_merge([0], array_column($apps, 'id'))) + 1;
            $app = buildAppRecord($nextId, false, $data);
            $apps[] = $app;

            return $app;
        });
    }

    /**
     * Changes the given fields and keeps the rest.
     *
     * @param array<string, mixed> $changes
     * @return array<string, mixed>
     */
    public function update(int $id, array $changes): array
    {
        return $this->change(function (array &$apps) use ($id, $changes): array {
            $index = self::indexOf($apps, $id);
            $current = $apps[$index];
            $apps[$index] = buildAppRecord($id, ($current['enabled'] ?? false) === true, array_merge(storedFieldsOf($current), $changes));

            return $apps[$index];
        });
    }

    /**
     * Enabling validates the record again, because the client rejects the
     * whole list when one enabled app is invalid.
     *
     * @return array<string, mixed>
     */
    public function setEnabled(int $id, bool $enabled): array
    {
        return $this->change(function (array &$apps) use ($id, $enabled): array {
            $index = self::indexOf($apps, $id);
            if ($enabled) {
                $apps[$index] = buildAppRecord($id, true, storedFieldsOf($apps[$index]));
            }
            $apps[$index]['enabled'] = $enabled;

            return $apps[$index];
        });
    }

    /**
     * @return array<string, mixed> the removed record
     */
    public function delete(int $id): array
    {
        return $this->change(function (array &$apps) use ($id): array {
            $index = self::indexOf($apps, $id);
            $removed = $apps[$index];
            array_splice($apps, $index, 1);

            return $removed;
        });
    }

    /**
     * Runs $change on the app list under an exclusive lock and saves the list.
     * Other top-level keys of apps.json are kept.
     *
     * @return mixed what $change returns
     */
    private function change(callable $change)
    {
        return withFileLock($this->lockPath, LOCK_EX, function () use ($change) {
            $document = $this->readDocument();
            $result = $change($document['apps']);
            writeJsonFile($this->filePath, $document);

            return $result;
        });
    }

    /**
     * @return array{apps: array<int, array<string, mixed>>}
     */
    private function readDocument(): array
    {
        $document = readJsonFile($this->filePath);
        if (!is_array($document['apps'] ?? null)) {
            throw new RuntimeException("Soubor {$this->filePath} musí obsahovat kořenový objekt s polem apps.");
        }

        $document['apps'] = array_values($document['apps']);
        $seenIds = [];
        foreach ($document['apps'] as $index => $app) {
            $id = $app['id'] ?? null;
            if (!is_int($id) || $id < 1 || isset($seenIds[$id])) {
                throw new RuntimeException("Aplikace na pozici {$index} v apps.json nemá jedinečné kladné celočíselné ID. Opravte apps.json ručně.");
            }
            $seenIds[$id] = true;
        }

        return $document;
    }

    /**
     * @param array<int, array<string, mixed>> $apps
     */
    private static function indexOf(array $apps, int $id): int
    {
        foreach ($apps as $index => $app) {
            if ($app['id'] === $id) {
                return $index;
            }
        }

        throw ApiError::notFound("Aplikace s ID {$id} nebyla nalezena.");
    }
}
