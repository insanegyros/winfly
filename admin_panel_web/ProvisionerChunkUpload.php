<?php

declare(strict_types=1);

/**
 * Chyba související s blokovým nahráváním instalátoru.
 */
final class ProvisionerChunkUploadException extends RuntimeException
{
    /** @var string */
    private $uploadCode;

    /** @var int */
    private $httpStatus;

    public function __construct(
        string $message,
        string $uploadCode = 'UPLOAD_ERROR',
        int $httpStatus = 500,
        ?Throwable $previous = null
    ) {
        parent::__construct($message, 0, $previous);
        $this->uploadCode = $uploadCode;
        $this->httpStatus = $httpStatus;
    }

    public function getUploadCode(): string
    {
        return $this->uploadCode;
    }

    public function getHttpStatus(): int
    {
        return $this->httpStatus;
    }
}

/**
 * Spravuje blokové nahrávání velkých EXE/MSI souborů.
 *
 * Každý blok se zapisuje přímo na svůj offset do předem připraveného
 * dočasného souboru. Na konci proto není nutné celý instalátor znovu
 * skládat nebo kopírovat v jednom dlouhém PHP požadavku.
 */
final class ProvisionerChunkUpload
{
    private const METADATA_FILE = 'metadata.json';
    private const PAYLOAD_FILE = 'payload.part';
    private const SESSION_LOCK_FILE = 'upload.lock';
    private const CANCELLED_FILE = 'cancelled';
    private const COMPLETED_DIRECTORY = 'completed';
    private const CHUNK_LOCK_DIRECTORY = 'chunk-locks';
    private const MINIMUM_ABSOLUTE_CHUNK_SIZE = 16384;

    /** @var string */
    private $stagingDirectory;

    /** @var string */
    private $installerDirectory;

    /** @var int */
    private $maximumChunkSize;

    /** @var int */
    private $minimumChunkSize;

    /** @var int */
    private $maxAgeSeconds;

    /** @var string */
    private $ownerHash;

    public function __construct(
        string $stagingDirectory,
        string $installerDirectory,
        string $ownerToken,
        int $maximumChunkSize = 1048576,
        int $minimumChunkSize = 65536,
        int $maxAgeSeconds = 86400
    ) {
        $stagingDirectory = rtrim(trim($stagingDirectory), DIRECTORY_SEPARATOR);
        $installerDirectory = rtrim(trim($installerDirectory), DIRECTORY_SEPARATOR);

        if ($stagingDirectory === '') {
            throw new RuntimeException('Dočasná složka pro blokové nahrávání není nastavena.');
        }

        if ($installerDirectory === '' || !is_dir($installerDirectory)) {
            throw new RuntimeException("Složka pro instalátory neexistuje: {$installerDirectory}");
        }

        if (!is_writable($installerDirectory)) {
            throw new RuntimeException("Do složky pro instalátory nelze zapisovat: {$installerDirectory}");
        }

        if ($ownerToken === '') {
            throw new RuntimeException('Nahrávací relaci nelze svázat s přihlášenou PHP session.');
        }

        $maximumChunkSize = max(self::MINIMUM_ABSOLUTE_CHUNK_SIZE, $maximumChunkSize);
        $minimumChunkSize = max(self::MINIMUM_ABSOLUTE_CHUNK_SIZE, $minimumChunkSize);

        $postLimit = self::parseIniBytes((string) ini_get('post_max_size'));
        if ($postLimit > 0) {
            // Ponechá dostatečnou rezervu pro servery, které uplatňují post_max_size
            // také na application/octet-stream.
            $safePostBody = max(
                self::MINIMUM_ABSOLUTE_CHUNK_SIZE,
                (int) floor($postLimit * 0.70)
            );
            $maximumChunkSize = min($maximumChunkSize, $safePostBody);
        }

        if ($minimumChunkSize > $maximumChunkSize) {
            $minimumChunkSize = $maximumChunkSize;
        }

        $this->stagingDirectory = $stagingDirectory;
        $this->installerDirectory = $installerDirectory;
        $this->maximumChunkSize = $maximumChunkSize;
        $this->minimumChunkSize = $minimumChunkSize;
        $this->maxAgeSeconds = max(300, $maxAgeSeconds);
        $this->ownerHash = hash('sha256', $ownerToken);

        $this->ensureStagingDirectory();
    }

    public function getMaximumChunkSize(): int
    {
        return $this->maximumChunkSize;
    }

    public function getMinimumChunkSize(): int
    {
        return $this->minimumChunkSize;
    }

    /**
     * @return array<string, mixed>
     */
    public function initialize(string $originalName, int $fileSize, ?int $requestedChunkSize = null): array
    {
        $validatedName = self::validateInstallerName($originalName);

        if ($fileSize < 1) {
            throw new ProvisionerChunkUploadException(
                'Instalační soubor je prázdný.',
                'EMPTY_FILE',
                422
            );
        }

        $destination = $this->installerDirectory . DIRECTORY_SEPARATOR . $validatedName['name'];
        if (file_exists($destination) || is_link($destination)) {
            throw new ProvisionerChunkUploadException(
                "Soubor {$validatedName['name']} už ve složce instalátorů existuje. " .
                'Přejmenujte nahrávaný soubor nebo nejprve odstraňte původní záznam.',
                'FILE_EXISTS',
                409
            );
        }

        $freeSpace = @disk_free_space($this->installerDirectory);
        if (is_float($freeSpace) && $freeSpace > 0 && $freeSpace < ($fileSize + 16777216)) {
            throw new ProvisionerChunkUploadException(
                'Na serveru není dostatek volného místa pro celý instalační soubor.',
                'INSUFFICIENT_STORAGE',
                507
            );
        }

        $chunkSize = $requestedChunkSize ?? $this->maximumChunkSize;
        $chunkSize = max($this->minimumChunkSize, min($this->maximumChunkSize, $chunkSize));
        $totalChunks = intdiv($fileSize - 1, $chunkSize) + 1;
        $uploadId = $this->createUploadId();
        $sessionDirectory = $this->sessionDirectory($uploadId);

        if (!@mkdir($sessionDirectory, 0700, true) && !is_dir($sessionDirectory)) {
            throw new RuntimeException('Dočasnou složku nahrávání se nepodařilo vytvořit.');
        }

        try {
            $this->createDirectory($sessionDirectory . DIRECTORY_SEPARATOR . self::COMPLETED_DIRECTORY);
            $this->createDirectory($sessionDirectory . DIRECTORY_SEPARATOR . self::CHUNK_LOCK_DIRECTORY);

            $lockHandle = @fopen(
                $sessionDirectory . DIRECTORY_SEPARATOR . self::SESSION_LOCK_FILE,
                'x+b'
            );
            if ($lockHandle === false) {
                throw new RuntimeException('Zámek nahrávání se nepodařilo vytvořit.');
            }
            fclose($lockHandle);

            $payloadPath = $sessionDirectory . DIRECTORY_SEPARATOR . self::PAYLOAD_FILE;
            $payloadHandle = @fopen($payloadPath, 'x+b');
            if ($payloadHandle === false) {
                throw new RuntimeException('Dočasný soubor instalátoru se nepodařilo vytvořit.');
            }

            try {
                if (!@ftruncate($payloadHandle, $fileSize)) {
                    throw new RuntimeException(
                        'Dočasný soubor instalátoru se nepodařilo připravit na požadovanou velikost.'
                    );
                }

                if (!fflush($payloadHandle)) {
                    throw new RuntimeException('Dočasný soubor instalátoru se nepodařilo zapsat.');
                }
            } finally {
                fclose($payloadHandle);
            }

            $metadata = [
                'version' => 1,
                'uploadId' => $uploadId,
                'ownerHash' => $this->ownerHash,
                'fileName' => $validatedName['name'],
                'extension' => $validatedName['extension'],
                'fileSize' => $fileSize,
                'chunkSize' => $chunkSize,
                'totalChunks' => $totalChunks,
                'createdAt' => time(),
            ];

            $this->writeJsonAtomically(
                $sessionDirectory . DIRECTORY_SEPARATOR . self::METADATA_FILE,
                $metadata
            );
        } catch (Throwable $exception) {
            $this->removeDirectory($sessionDirectory);
            throw $exception;
        }

        return [
            'uploadId' => $uploadId,
            'fileName' => $validatedName['name'],
            'fileSize' => $fileSize,
            'extension' => $validatedName['extension'],
            'chunkSizeBytes' => $chunkSize,
            'minimumChunkSizeBytes' => $this->minimumChunkSize,
            'totalChunks' => $totalChunks,
        ];
    }

    /**
     * @param resource $inputStream
     * @return array<string, mixed>
     */
    public function writeChunk(
        string $uploadId,
        int $chunkIndex,
        $inputStream,
        ?int $contentLength = null
    ): array {
        if (!is_resource($inputStream)) {
            throw new RuntimeException('Datový proud nahrávaného bloku není dostupný.');
        }

        $uploadId = $this->normalizeUploadId($uploadId);
        $sessionDirectory = $this->sessionDirectory($uploadId);
        $sessionLock = $this->openSessionLock($sessionDirectory, LOCK_SH);

        try {
            $metadata = $this->readOwnedMetadata($sessionDirectory);
            $totalChunks = (int) $metadata['totalChunks'];

            if ($chunkIndex < 0 || $chunkIndex >= $totalChunks) {
                throw new ProvisionerChunkUploadException(
                    'Pořadové číslo nahrávaného bloku je mimo platný rozsah.',
                    'INVALID_CHUNK_INDEX',
                    422
                );
            }

            $chunkSize = (int) $metadata['chunkSize'];
            $fileSize = (int) $metadata['fileSize'];
            $offset = $chunkIndex * $chunkSize;
            $expectedBytes = min($chunkSize, $fileSize - $offset);

            if ($contentLength !== null && $contentLength !== $expectedBytes) {
                throw new ProvisionerChunkUploadException(
                    "Blok {$chunkIndex} má velikost {$contentLength} B, očekáváno je {$expectedBytes} B.",
                    'CHUNK_BODY_SIZE',
                    413
                );
            }

            $markerPath = $this->chunkMarkerPath($sessionDirectory, $chunkIndex);
            $chunkLockPath = $this->chunkLockPath($sessionDirectory, $chunkIndex);
            $chunkLock = @fopen($chunkLockPath, 'c+b');
            if ($chunkLock === false || !flock($chunkLock, LOCK_EX)) {
                if (is_resource($chunkLock)) {
                    fclose($chunkLock);
                }
                throw new RuntimeException("Blok {$chunkIndex} se nepodařilo uzamknout.");
            }

            try {
                if (is_file($markerPath)) {
                    return [
                        'uploadId' => $uploadId,
                        'chunkIndex' => $chunkIndex,
                        'receivedBytes' => $expectedBytes,
                        'duplicate' => true,
                    ];
                }

                $payloadPath = $sessionDirectory . DIRECTORY_SEPARATOR . self::PAYLOAD_FILE;
                $payloadHandle = @fopen($payloadPath, 'c+b');
                if ($payloadHandle === false) {
                    throw new RuntimeException('Dočasný soubor instalátoru se nepodařilo otevřít.');
                }

                try {
                    if (fseek($payloadHandle, $offset, SEEK_SET) !== 0) {
                        throw new RuntimeException("Dočasný soubor nelze nastavit na blok {$chunkIndex}.");
                    }

                    $remaining = $expectedBytes;
                    while ($remaining > 0) {
                        $buffer = fread($inputStream, min(1048576, $remaining));
                        if ($buffer === false) {
                            throw new RuntimeException("Blok {$chunkIndex} se nepodařilo načíst z požadavku.");
                        }

                        if ($buffer === '') {
                            if (feof($inputStream)) {
                                break;
                            }
                            continue;
                        }

                        $this->writeAll($payloadHandle, $buffer);
                        $remaining -= strlen($buffer);
                    }

                    $extraByte = fread($inputStream, 1);
                    if ($remaining !== 0 || ($extraByte !== false && $extraByte !== '')) {
                        $received = $expectedBytes - $remaining + (($extraByte !== false && $extraByte !== '') ? 1 : 0);
                        throw new ProvisionerChunkUploadException(
                            "Blok {$chunkIndex} nebyl přijat celý nebo obsahoval data navíc " .
                            "({$received} B, očekáváno {$expectedBytes} B).",
                            'CHUNK_BODY_SIZE',
                            413
                        );
                    }

                    if (!fflush($payloadHandle)) {
                        throw new RuntimeException("Blok {$chunkIndex} se nepodařilo zapsat na disk.");
                    }

                } finally {
                    fclose($payloadHandle);
                }

                $markerHandle = @fopen($markerPath, 'xb');
                if ($markerHandle === false) {
                    if (!is_file($markerPath)) {
                        throw new RuntimeException("Potvrzení bloku {$chunkIndex} se nepodařilo vytvořit.");
                    }
                } else {
                    fwrite($markerHandle, (string) $expectedBytes);
                    fflush($markerHandle);
                    fclose($markerHandle);
                }

                @touch($sessionDirectory . DIRECTORY_SEPARATOR . self::METADATA_FILE);

                return [
                    'uploadId' => $uploadId,
                    'chunkIndex' => $chunkIndex,
                    'receivedBytes' => $expectedBytes,
                    'duplicate' => false,
                ];
            } finally {
                flock($chunkLock, LOCK_UN);
                fclose($chunkLock);
            }
        } finally {
            $this->releaseLock($sessionLock);
        }
    }

    /**
     * Přesune hotový instalátor atomicky do cílové složky a uvnitř stejného
     * kroku zavolá callback, který vytvoří záznam v apps.json. Při chybě
     * callbacku se soubor přesune zpět do nahrávací relace.
     *
     * @return mixed
     */
    public function complete(string $uploadId, callable $callback)
    {
        $uploadId = $this->normalizeUploadId($uploadId);
        $sessionDirectory = $this->sessionDirectory($uploadId);
        $sessionLock = $this->openSessionLock($sessionDirectory, LOCK_EX);
        $completed = false;

        try {
            $metadata = $this->readOwnedMetadata($sessionDirectory);
            $totalChunks = (int) $metadata['totalChunks'];
            $missingChunks = [];

            for ($index = 0; $index < $totalChunks; ++$index) {
                if (!is_file($this->chunkMarkerPath($sessionDirectory, $index))) {
                    $missingChunks[] = $index;
                    if (count($missingChunks) >= 20) {
                        break;
                    }
                }
            }

            if ($missingChunks !== []) {
                throw new ProvisionerChunkUploadException(
                    'Nahrávání ještě není dokončeno. Chybí bloky: ' . implode(', ', $missingChunks) . '.',
                    'MISSING_CHUNKS',
                    409
                );
            }

            $payloadPath = $sessionDirectory . DIRECTORY_SEPARATOR . self::PAYLOAD_FILE;
            clearstatcache(true, $payloadPath);
            $actualSize = @filesize($payloadPath);
            $expectedSize = (int) $metadata['fileSize'];

            if (!is_int($actualSize) || $actualSize !== $expectedSize) {
                throw new ProvisionerChunkUploadException(
                    'Dočasný instalační soubor nemá po nahrání očekávanou velikost.',
                    'FINAL_SIZE_MISMATCH',
                    409
                );
            }

            $fileName = (string) $metadata['fileName'];
            $destination = $this->installerDirectory . DIRECTORY_SEPARATOR . $fileName;

            if (file_exists($destination) || is_link($destination)) {
                throw new ProvisionerChunkUploadException(
                    "Soubor {$fileName} už ve složce instalátorů existuje.",
                    'FILE_EXISTS',
                    409
                );
            }

            if (!@rename($payloadPath, $destination)) {
                throw new RuntimeException(
                    'Hotový instalátor se nepodařilo atomicky přesunout do cílové složky. ' .
                    'Dočasná a cílová složka musí být na stejném souborovém systému.'
                );
            }

            @chmod($destination, 0664);

            $storedInstaller = [
                'name' => $fileName,
                'path' => $destination,
                'extension' => (string) $metadata['extension'],
                'size' => $expectedSize,
            ];

            try {
                $result = $callback($storedInstaller);
            } catch (Throwable $exception) {
                if (!@rename($destination, $payloadPath)) {
                    throw new RuntimeException(
                        'Záznam aplikace se nepodařilo vytvořit a instalátor se zároveň nepodařilo ' .
                        'vrátit do dočasné nahrávací relace. Zkontrolujte cílovou složku. ' .
                        'Původní chyba: ' . $exception->getMessage(),
                        0,
                        $exception
                    );
                }

                throw $exception;
            }

            $completed = true;
            return $result;
        } finally {
            $this->releaseLock($sessionLock);

            if ($completed) {
                $this->removeDirectory($sessionDirectory);
            }
        }
    }

    public function cancel(string $uploadId): bool
    {
        $uploadId = $this->normalizeUploadId($uploadId);
        $sessionDirectory = $this->sessionDirectory($uploadId);

        if (!is_dir($sessionDirectory)) {
            return false;
        }

        $sessionLock = $this->openSessionLock($sessionDirectory, LOCK_EX);

        try {
            $this->readOwnedMetadata($sessionDirectory);
            @file_put_contents(
                $sessionDirectory . DIRECTORY_SEPARATOR . self::CANCELLED_FILE,
                (string) time(),
                LOCK_EX
            );
        } finally {
            $this->releaseLock($sessionLock);
        }

        $this->removeDirectory($sessionDirectory);
        return true;
    }

    public function cleanupExpired(): void
    {
        $items = @scandir($this->stagingDirectory);
        if (!is_array($items)) {
            return;
        }

        $threshold = time() - $this->maxAgeSeconds;

        foreach ($items as $item) {
            if (preg_match('/^[a-f0-9]{32}$/D', $item) !== 1) {
                continue;
            }

            $sessionDirectory = $this->stagingDirectory . DIRECTORY_SEPARATOR . $item;
            $metadataPath = $sessionDirectory . DIRECTORY_SEPARATOR . self::METADATA_FILE;
            $modifiedAt = @filemtime($metadataPath);

            if (!is_int($modifiedAt) || $modifiedAt >= $threshold) {
                continue;
            }

            $lockPath = $sessionDirectory . DIRECTORY_SEPARATOR . self::SESSION_LOCK_FILE;
            $lockHandle = @fopen($lockPath, 'c+b');
            if ($lockHandle === false) {
                continue;
            }

            if (!flock($lockHandle, LOCK_EX | LOCK_NB)) {
                fclose($lockHandle);
                continue;
            }

            @file_put_contents(
                $sessionDirectory . DIRECTORY_SEPARATOR . self::CANCELLED_FILE,
                (string) time(),
                LOCK_EX
            );
            flock($lockHandle, LOCK_UN);
            fclose($lockHandle);
            $this->removeDirectory($sessionDirectory);
        }
    }

    /**
     * @return array{name: string, extension: string}
     */
    public static function validateInstallerName(string $originalName): array
    {
        $name = basename(str_replace('\\', '/', trim($originalName)));

        if ($name === '' || $name === '.' || $name === '..') {
            throw new ProvisionerChunkUploadException(
                'Název nahrávaného instalačního souboru není platný.',
                'INVALID_FILE_NAME',
                422
            );
        }

        if ($name !== trim($name, " .")) {
            throw new ProvisionerChunkUploadException(
                'Název instalačního souboru nesmí začínat ani končit tečkou nebo mezerou.',
                'INVALID_FILE_NAME',
                422
            );
        }

        if (
            strpos($name, "\0") !== false ||
            preg_match('/[<>:"|?*\/\\\\\x00-\x1F\x7F]/u', $name) === 1
        ) {
            throw new ProvisionerChunkUploadException(
                'Název instalačního souboru obsahuje znak, který nelze bezpečně použít.',
                'INVALID_FILE_NAME',
                422
            );
        }

        if (strlen($name) > 240) {
            throw new ProvisionerChunkUploadException(
                'Název instalačního souboru je příliš dlouhý.',
                'INVALID_FILE_NAME',
                422
            );
        }

        $extension = strtolower((string) pathinfo($name, PATHINFO_EXTENSION));
        if (!in_array($extension, ['exe', 'msi'], true)) {
            throw new ProvisionerChunkUploadException(
                'Nahrát lze pouze instalační soubor s příponou .exe nebo .msi.',
                'INVALID_FILE_TYPE',
                422
            );
        }

        return [
            'name' => $name,
            'extension' => $extension,
        ];
    }

    private function ensureStagingDirectory(): void
    {
        if (!is_dir($this->stagingDirectory)) {
            if (!@mkdir($this->stagingDirectory, 0700, true) && !is_dir($this->stagingDirectory)) {
                throw new RuntimeException(
                    "Dočasnou složku pro blokové nahrávání nelze vytvořit: {$this->stagingDirectory}"
                );
            }
        }

        if (!is_writable($this->stagingDirectory)) {
            throw new RuntimeException(
                "Do dočasné složky pro blokové nahrávání nelze zapisovat: {$this->stagingDirectory}"
            );
        }

        $htaccessPath = $this->stagingDirectory . DIRECTORY_SEPARATOR . '.htaccess';
        if (!file_exists($htaccessPath)) {
            @file_put_contents(
                $htaccessPath,
                "Require all denied\nDeny from all\n",
                LOCK_EX
            );
        }

        $indexPath = $this->stagingDirectory . DIRECTORY_SEPARATOR . 'index.html';
        if (!file_exists($indexPath)) {
            @file_put_contents($indexPath, '', LOCK_EX);
        }
    }

    private function createDirectory(string $path): void
    {
        if (!@mkdir($path, 0700, true) && !is_dir($path)) {
            throw new RuntimeException("Dočasnou složku nelze vytvořit: {$path}");
        }
    }

    private function createUploadId(): string
    {
        for ($attempt = 0; $attempt < 20; ++$attempt) {
            $uploadId = bin2hex(random_bytes(16));
            if (!file_exists($this->sessionDirectory($uploadId))) {
                return $uploadId;
            }
        }

        throw new RuntimeException('Jedinečné ID nahrávání se nepodařilo vytvořit.');
    }

    private function normalizeUploadId(string $uploadId): string
    {
        $uploadId = strtolower(trim($uploadId));
        if (preg_match('/^[a-f0-9]{32}$/D', $uploadId) !== 1) {
            throw new ProvisionerChunkUploadException(
                'ID blokového nahrávání není platné.',
                'INVALID_UPLOAD_ID',
                422
            );
        }

        return $uploadId;
    }

    private function sessionDirectory(string $uploadId): string
    {
        return $this->stagingDirectory . DIRECTORY_SEPARATOR . $uploadId;
    }

    /**
     * @return resource
     */
    private function openSessionLock(string $sessionDirectory, int $mode)
    {
        if (!is_dir($sessionDirectory)) {
            throw new ProvisionerChunkUploadException(
                'Nahrávací relace neexistuje nebo už vypršela.',
                'UPLOAD_NOT_FOUND',
                404
            );
        }

        $lockPath = $sessionDirectory . DIRECTORY_SEPARATOR . self::SESSION_LOCK_FILE;
        $lockHandle = @fopen($lockPath, 'c+b');
        if ($lockHandle === false || !flock($lockHandle, $mode)) {
            if (is_resource($lockHandle)) {
                fclose($lockHandle);
            }
            throw new RuntimeException('Nahrávací relaci se nepodařilo uzamknout.');
        }

        if (is_file($sessionDirectory . DIRECTORY_SEPARATOR . self::CANCELLED_FILE)) {
            $this->releaseLock($lockHandle);
            throw new ProvisionerChunkUploadException(
                'Nahrávání bylo zrušeno.',
                'UPLOAD_CANCELLED',
                409
            );
        }

        return $lockHandle;
    }

    /**
     * @param resource $lockHandle
     */
    private function releaseLock($lockHandle): void
    {
        if (is_resource($lockHandle)) {
            flock($lockHandle, LOCK_UN);
            fclose($lockHandle);
        }
    }

    /**
     * @return array<string, mixed>
     */
    private function readOwnedMetadata(string $sessionDirectory): array
    {
        $metadataPath = $sessionDirectory . DIRECTORY_SEPARATOR . self::METADATA_FILE;
        $raw = @file_get_contents($metadataPath);
        if (!is_string($raw) || trim($raw) === '') {
            throw new RuntimeException('Metadata nahrávací relace nejsou dostupná.');
        }

        try {
            $metadata = json_decode($raw, true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $exception) {
            throw new RuntimeException('Metadata nahrávací relace jsou poškozená.', 0, $exception);
        }

        if (!is_array($metadata)) {
            throw new RuntimeException('Metadata nahrávací relace mají neplatnou strukturu.');
        }

        foreach (['uploadId', 'ownerHash', 'fileName', 'extension'] as $key) {
            if (!isset($metadata[$key]) || !is_string($metadata[$key])) {
                throw new RuntimeException("V metadatech nahrávání chybí hodnota {$key}.");
            }
        }

        foreach (['fileSize', 'chunkSize', 'totalChunks', 'createdAt'] as $key) {
            if (!isset($metadata[$key]) || !is_int($metadata[$key]) || $metadata[$key] < 1) {
                throw new RuntimeException("V metadatech nahrávání chybí platná hodnota {$key}.");
            }
        }

        if (!hash_equals($this->ownerHash, $metadata['ownerHash'])) {
            throw new ProvisionerChunkUploadException(
                'Nahrávací relace patří jiné přihlášené session.',
                'UPLOAD_OWNER_MISMATCH',
                403
            );
        }

        return $metadata;
    }

    private function chunkMarkerPath(string $sessionDirectory, int $chunkIndex): string
    {
        return $sessionDirectory
            . DIRECTORY_SEPARATOR
            . self::COMPLETED_DIRECTORY
            . DIRECTORY_SEPARATOR
            . $chunkIndex
            . '.done';
    }

    private function chunkLockPath(string $sessionDirectory, int $chunkIndex): string
    {
        return $sessionDirectory
            . DIRECTORY_SEPARATOR
            . self::CHUNK_LOCK_DIRECTORY
            . DIRECTORY_SEPARATOR
            . $chunkIndex
            . '.lock';
    }

    /**
     * @param resource $handle
     */
    private function writeAll($handle, string $data): void
    {
        $length = strlen($data);
        $offset = 0;

        while ($offset < $length) {
            $written = fwrite($handle, substr($data, $offset));
            if ($written === false || $written === 0) {
                throw new RuntimeException('Část instalačního souboru se nepodařilo zapsat na disk.');
            }
            $offset += $written;
        }
    }

    /**
     * @param array<string, mixed> $data
     */
    private function writeJsonAtomically(string $path, array $data): void
    {
        try {
            $json = json_encode(
                $data,
                JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE | JSON_THROW_ON_ERROR
            );
        } catch (JsonException $exception) {
            throw new RuntimeException('Metadata nahrávání se nepodařilo převést do JSON.', 0, $exception);
        }

        $temporaryPath = $path . '.tmp-' . bin2hex(random_bytes(8));
        $handle = @fopen($temporaryPath, 'xb');
        if ($handle === false) {
            throw new RuntimeException('Dočasná metadata nahrávání se nepodařilo vytvořit.');
        }

        try {
            $this->writeAll($handle, $json . PHP_EOL);
            if (!fflush($handle)) {
                throw new RuntimeException('Metadata nahrávání se nepodařilo zapsat na disk.');
            }
            if (function_exists('fsync')) {
                @fsync($handle);
            }
        } catch (Throwable $exception) {
            fclose($handle);
            @unlink($temporaryPath);
            throw $exception;
        }

        fclose($handle);

        if (!@rename($temporaryPath, $path)) {
            @unlink($temporaryPath);
            throw new RuntimeException('Metadata nahrávání se nepodařilo bezpečně uložit.');
        }
    }

    private function removeDirectory(string $directory): void
    {
        if (!file_exists($directory) && !is_link($directory)) {
            return;
        }

        if (is_link($directory) || !is_dir($directory)) {
            @unlink($directory);
            return;
        }

        $items = @scandir($directory);
        if (is_array($items)) {
            foreach ($items as $item) {
                if ($item === '.' || $item === '..') {
                    continue;
                }

                $path = $directory . DIRECTORY_SEPARATOR . $item;
                if (is_dir($path) && !is_link($path)) {
                    $this->removeDirectory($path);
                } else {
                    @unlink($path);
                }
            }
        }

        @rmdir($directory);
    }

    private static function parseIniBytes(string $value): int
    {
        $value = trim($value);
        if ($value === '' || $value === '-1') {
            return 0;
        }

        if (preg_match('/^([0-9]+(?:\.[0-9]+)?)\s*([KMGTP]?)B?$/i', $value, $matches) !== 1) {
            return 0;
        }

        $number = (float) $matches[1];
        $unit = strtoupper($matches[2]);
        $powers = [
            '' => 0,
            'K' => 1,
            'M' => 2,
            'G' => 3,
            'T' => 4,
            'P' => 5,
        ];
        $power = $powers[$unit] ?? 0;
        $bytes = $number * (1024 ** $power);

        if ($bytes >= PHP_INT_MAX) {
            return PHP_INT_MAX;
        }

        return (int) floor($bytes);
    }
}
