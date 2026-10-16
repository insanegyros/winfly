<?php

declare(strict_types=1);

/**
 * Uploads a large installer in small chunks, so the file can be bigger than
 * the server's upload_max_filesize and post_max_size.
 *
 * Every upload gets its own folder in the staging directory:
 *   metadata.json   file name, size, chunk size, owner
 *   payload.part    the installer, pre-sized; each chunk is written at its offset
 *   chunks/N.done   marker of a received chunk (N.lock guards one chunk)
 *   upload.lock     shared while chunks arrive, exclusive to finish or cancel
 *   cancelled       marker that stops chunks still on their way
 *
 * Finishing moves payload.part into the installer folder with one rename.
 */
final class ChunkUpload
{
    private const MINIMUM_CHUNK_SIZE = 16384;

    private string $stagingDirectory;
    private InstallerStorage $storage;
    private string $ownerHash;
    private int $maximumChunkSize;
    private int $minimumChunkSize;
    private int $maxAgeSeconds;

    /**
     * @param array{chunkUploadDirectory: string, chunkSizeBytes: int, chunkMinimumSizeBytes: int, chunkUploadMaxAgeSeconds: int} $settings
     * @param string $ownerToken the PHP session ID; an upload only accepts chunks from the session that started it
     */
    public function __construct(array $settings, InstallerStorage $storage, string $ownerToken)
    {
        if (!is_dir($storage->directory()) || !is_writable($storage->directory())) {
            throw new RuntimeException('Do složky pro instalátory nelze zapisovat: ' . $storage->directory());
        }

        // Leave headroom for servers that apply post_max_size to raw request bodies too.
        $postLimit = iniSizeToBytes((string) ini_get('post_max_size'));
        $maximumChunkSize = $settings['chunkSizeBytes'];
        if ($postLimit > 0) {
            $maximumChunkSize = min($maximumChunkSize, max(self::MINIMUM_CHUNK_SIZE, (int) ($postLimit * 0.7)));
        }

        $this->stagingDirectory = rtrim($settings['chunkUploadDirectory'], '/\\');
        $this->storage = $storage;
        $this->ownerHash = hash('sha256', $ownerToken);
        $this->maximumChunkSize = $maximumChunkSize;
        $this->minimumChunkSize = min($settings['chunkMinimumSizeBytes'], $maximumChunkSize);
        $this->maxAgeSeconds = $settings['chunkUploadMaxAgeSeconds'];

        $this->prepareStagingDirectory();
    }

    /**
     * Starts an upload and returns what the browser needs to send the chunks.
     *
     * @return array<string, mixed>
     */
    public function start(string $originalName, int $fileSize, ?int $requestedChunkSize): array
    {
        $this->removeExpiredUploads();

        $fileName = basename(str_replace('\\', '/', trim($originalName)));
        assertSafeFileName($fileName, 'Název nahrávaného souboru');
        $extension = installerTypeFromFileName($fileName);
        if ($extension === null) {
            throw new ApiError('Nahrát lze pouze soubor s příponou .exe, .msi, .iso nebo .img.', 422, 'INVALID_FILE_TYPE');
        }

        $this->assertDestinationIsFree($fileName);
        $freeSpace = @disk_free_space($this->storage->directory());
        if (is_float($freeSpace) && $freeSpace > 0 && $freeSpace < $fileSize + 16777216) {
            throw new ApiError('Na serveru není dostatek volného místa pro celý instalační soubor.', 507, 'INSUFFICIENT_STORAGE');
        }

        $chunkSize = max($this->minimumChunkSize, min($this->maximumChunkSize, $requestedChunkSize ?? $this->maximumChunkSize));
        $uploadId = bin2hex(random_bytes(16));
        $directory = $this->uploadDirectory($uploadId);
        $metadata = [
            'version' => 2,
            'ownerHash' => $this->ownerHash,
            'fileName' => $fileName,
            'extension' => $extension,
            'fileSize' => $fileSize,
            'chunkSize' => $chunkSize,
            'totalChunks' => intdiv($fileSize - 1, $chunkSize) + 1,
        ];

        try {
            createDirectory($directory . '/chunks');
            $this->createPayloadFile($directory . '/payload.part', $fileSize);
            writeJsonFile($directory . '/metadata.json', $metadata);
        } catch (Throwable $exception) {
            removeDirectory($directory);
            throw $exception;
        }

        return [
            'uploadId' => $uploadId,
            'fileName' => $fileName,
            'fileSize' => $fileSize,
            'extension' => $extension,
            'chunkSizeBytes' => $chunkSize,
            'minimumChunkSizeBytes' => $this->minimumChunkSize,
            'totalChunks' => $metadata['totalChunks'],
        ];
    }

    /**
     * Writes one chunk from $input at its place in the payload. Sending the same
     * chunk twice is harmless.
     *
     * @param resource $input
     * @return array<string, mixed>
     */
    public function receiveChunk(string $uploadId, int $chunkIndex, $input, ?int $contentLength): array
    {
        return $this->withUpload($uploadId, LOCK_SH, function (string $directory, array $metadata) use ($chunkIndex, $input, $contentLength): array {
            if ($chunkIndex >= $metadata['totalChunks']) {
                throw new ApiError('Pořadové číslo nahrávaného bloku je mimo platný rozsah.', 422, 'INVALID_CHUNK_INDEX');
            }

            $offset = $chunkIndex * $metadata['chunkSize'];
            $expectedBytes = min($metadata['chunkSize'], $metadata['fileSize'] - $offset);
            if ($contentLength !== null && $contentLength !== $expectedBytes) {
                throw $this->chunkSizeError($chunkIndex, $contentLength, $expectedBytes);
            }

            $result = ['chunkIndex' => $chunkIndex, 'receivedBytes' => $expectedBytes, 'duplicate' => true];
            $markerPath = "{$directory}/chunks/{$chunkIndex}.done";

            return withFileLock("{$directory}/chunks/{$chunkIndex}.lock", LOCK_EX, function () use ($directory, $markerPath, $input, $offset, $expectedBytes, $chunkIndex, $result): array {
                if (is_file($markerPath)) {
                    return $result;
                }

                $this->copyChunkIntoPayload($input, "{$directory}/payload.part", $offset, $expectedBytes, $chunkIndex);
                if (@file_put_contents($markerPath, (string) $expectedBytes) === false) {
                    throw new RuntimeException("Potvrzení bloku {$chunkIndex} se nepodařilo uložit.");
                }
                @touch("{$directory}/metadata.json");

                return ['duplicate' => false] + $result;
            });
        });
    }

    /**
     * Moves the finished installer into the installer folder and calls
     * $createApp with its file name. When $createApp fails, the
     * file is moved back so the upload can be finished again or cancelled.
     *
     * @return mixed what $createApp returns
     */
    public function finish(string $uploadId, callable $createApp)
    {
        $result = $this->withUpload($uploadId, LOCK_EX, function (string $directory, array $metadata) use ($createApp) {
            $missingChunks = [];
            for ($index = 0; $index < $metadata['totalChunks'] && count($missingChunks) < 20; ++$index) {
                if (!is_file("{$directory}/chunks/{$index}.done")) {
                    $missingChunks[] = $index;
                }
            }
            if ($missingChunks !== []) {
                throw ApiError::conflict('Nahrávání ještě není dokončeno. Chybí bloky: ' . implode(', ', $missingChunks) . '.', 'MISSING_CHUNKS');
            }

            $payloadPath = "{$directory}/payload.part";
            clearstatcache(true, $payloadPath);
            if (@filesize($payloadPath) !== $metadata['fileSize']) {
                throw ApiError::conflict('Dočasný instalační soubor nemá po nahrání očekávanou velikost.', 'FINAL_SIZE_MISMATCH');
            }

            $fileName = $metadata['fileName'];
            $destination = $this->storage->pathOf($fileName);
            $this->assertDestinationIsFree($fileName);
            if (!@rename($payloadPath, $destination)) {
                throw new RuntimeException('Hotový instalátor se nepodařilo přesunout do cílové složky. Dočasná a cílová složka musí být na stejném disku.');
            }
            @chmod($destination, 0664);

            try {
                return $createApp($fileName);
            } catch (Throwable $exception) {
                if (!@rename($destination, $payloadPath)) {
                    throw new RuntimeException('Záznam aplikace se nepodařilo vytvořit a instalátor se nepodařilo vrátit do dočasné složky. Původní chyba: ' . $exception->getMessage(), 0, $exception);
                }
                throw $exception;
            }
        });

        removeDirectory($this->uploadDirectory($uploadId));

        return $result;
    }

    /**
     * Stops an upload and removes its temporary data. Returns false when the
     * upload no longer existed.
     */
    public function cancel(string $uploadId): bool
    {
        if (!is_dir($this->uploadDirectory($this->validUploadId($uploadId)))) {
            return false;
        }

        $this->withUpload($uploadId, LOCK_EX, function (string $directory): void {
            @file_put_contents("{$directory}/cancelled", (string) time());
        });
        removeDirectory($this->uploadDirectory($uploadId));

        return true;
    }

    /**
     * Runs $work($directory, $metadata) under the upload's lock, after checking
     * that the upload exists, is not cancelled and belongs to this session.
     *
     * @return mixed what $work returns
     */
    private function withUpload(string $uploadId, int $lockMode, callable $work)
    {
        $directory = $this->uploadDirectory($this->validUploadId($uploadId));
        if (!is_file("{$directory}/metadata.json")) {
            throw ApiError::notFound('Nahrávací relace neexistuje nebo už vypršela.');
        }

        return withFileLock("{$directory}/upload.lock", $lockMode, function () use ($directory, $work) {
            if (is_file("{$directory}/cancelled")) {
                throw ApiError::conflict('Nahrávání bylo zrušeno.', 'UPLOAD_CANCELLED');
            }

            $metadata = readJsonFile("{$directory}/metadata.json");
            if (!hash_equals($this->ownerHash, (string) ($metadata['ownerHash'] ?? ''))) {
                throw new ApiError('Nahrávací relace patří jiné přihlášené session.', 403, 'UPLOAD_OWNER_MISMATCH');
            }

            return $work($directory, $metadata);
        });
    }

    /**
     * @param resource $input
     */
    private function copyChunkIntoPayload($input, string $payloadPath, int $offset, int $expectedBytes, int $chunkIndex): void
    {
        $payload = @fopen($payloadPath, 'c+b');
        if ($payload === false || fseek($payload, $offset) !== 0) {
            throw new RuntimeException("Dočasný soubor instalátoru nelze otevřít na pozici bloku {$chunkIndex}.");
        }

        try {
            $remaining = $expectedBytes;
            while ($remaining > 0 && !feof($input)) {
                $buffer = fread($input, min(1048576, $remaining));
                if ($buffer === false) {
                    throw new RuntimeException("Blok {$chunkIndex} se nepodařilo načíst z požadavku.");
                }
                writeAllBytes($payload, $buffer);
                $remaining -= strlen($buffer);
            }
        } finally {
            fclose($payload);
        }

        $receivedBytes = $expectedBytes - $remaining + strlen((string) fread($input, 1));
        if ($receivedBytes !== $expectedBytes) {
            throw $this->chunkSizeError($chunkIndex, $receivedBytes, $expectedBytes);
        }
    }

    private function chunkSizeError(int $chunkIndex, int $receivedBytes, int $expectedBytes): ApiError
    {
        return new ApiError("Blok {$chunkIndex} má velikost {$receivedBytes} B, očekáváno je {$expectedBytes} B.", 413, 'CHUNK_BODY_SIZE');
    }

    private function createPayloadFile(string $path, int $fileSize): void
    {
        $handle = @fopen($path, 'xb');
        if ($handle === false) {
            throw new RuntimeException('Dočasný soubor instalátoru se nepodařilo vytvořit.');
        }

        try {
            if (!@ftruncate($handle, $fileSize)) {
                throw new RuntimeException('Dočasný soubor instalátoru se nepodařilo připravit na požadovanou velikost.');
            }
        } finally {
            fclose($handle);
        }
    }

    private function assertDestinationIsFree(string $fileName): void
    {
        if (pathExists($this->storage->pathOf($fileName))) {
            throw ApiError::conflict("Soubor {$fileName} už ve složce instalátorů existuje. Přejmenujte nahrávaný soubor nebo nejprve smažte původní aplikaci.", 'FILE_EXISTS');
        }
    }

    /**
     * Removes uploads that received no chunk for longer than chunkUploadMaxAgeSeconds.
     */
    private function removeExpiredUploads(): void
    {
        $expiredBefore = time() - $this->maxAgeSeconds;

        foreach (scandir($this->stagingDirectory) ?: [] as $uploadId) {
            $directory = $this->uploadDirectory($uploadId);
            $lastActivity = @filemtime("{$directory}/metadata.json");
            if (preg_match('/^[a-f0-9]{32}$/D', $uploadId) !== 1 || $lastActivity === false || $lastActivity >= $expiredBefore) {
                continue;
            }

            try {
                withFileLock("{$directory}/upload.lock", LOCK_EX | LOCK_NB, function () use ($directory): void {
                    @file_put_contents("{$directory}/cancelled", (string) time());
                });
                removeDirectory($directory);
            } catch (RuntimeException $exception) {
                // Still in use by a running request; the next cleanup removes it.
            }
        }
    }

    private function prepareStagingDirectory(): void
    {
        createDirectory($this->stagingDirectory);
        if (!is_writable($this->stagingDirectory)) {
            throw new RuntimeException("Do dočasné složky pro nahrávání nelze zapisovat: {$this->stagingDirectory}");
        }

        // Keep the temporary files private on Apache, and hide the listing elsewhere.
        $protectionFiles = ['.htaccess' => "Require all denied\nDeny from all\n", 'index.html' => ''];
        foreach ($protectionFiles as $name => $content) {
            if (!file_exists("{$this->stagingDirectory}/{$name}")) {
                @file_put_contents("{$this->stagingDirectory}/{$name}", $content);
            }
        }
    }

    private function validUploadId(string $uploadId): string
    {
        $uploadId = strtolower(trim($uploadId));
        if (preg_match('/^[a-f0-9]{32}$/D', $uploadId) !== 1) {
            throw new ApiError('ID nahrávání není platné.', 422, 'INVALID_UPLOAD_ID');
        }

        return $uploadId;
    }

    private function uploadDirectory(string $uploadId): string
    {
        return "{$this->stagingDirectory}/{$uploadId}";
    }
}

/**
 * Converts a php.ini size such as "8M" to bytes; 0 means "no limit or unknown".
 */
function iniSizeToBytes(string $value): int
{
    if (preg_match('/^\s*(\d+(?:\.\d+)?)\s*([KMGTP]?)B?\s*$/i', $value, $matches) !== 1) {
        return 0;
    }

    $power = (int) strpos(' KMGTP', strtoupper($matches[2] !== '' ? $matches[2] : ' '));

    return (int) min(PHP_INT_MAX, floor((float) $matches[1] * 1024 ** $power));
}
