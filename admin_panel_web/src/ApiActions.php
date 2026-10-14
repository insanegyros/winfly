<?php

declare(strict_types=1);

/**
 * One public method per API action. Each returns the data of a successful
 * response; failures are thrown as ApiError.
 */
final class ApiActions
{
    /** Action name => [HTTP method, method of this class]. */
    public const ROUTES = [
        'list' => ['GET', 'listApps'],
        'get' => ['GET', 'getApp'],
        'update' => ['POST', 'updateApp'],
        'enable' => ['POST', 'enableApp'],
        'disable' => ['POST', 'disableApp'],
        'delete' => ['POST', 'deleteApp'],
        'upload-init' => ['POST', 'startUpload'],
        'upload-chunk' => ['POST', 'receiveChunk'],
        'upload-complete' => ['POST', 'finishUploadAndCreateApp'],
        'upload-cancel' => ['POST', 'cancelUpload'],
    ];

    /** @var array<string, mixed> */
    private array $settings;
    private string $sessionId;
    private AppRepository $apps;
    private InstallerStorage $storage;

    /**
     * @param array<string, mixed> $settings from loadSettings()
     */
    public function __construct(array $settings, string $sessionId)
    {
        $this->settings = $settings;
        $this->sessionId = $sessionId;
        $this->apps = new AppRepository($settings['appsFile']);
        $this->storage = new InstallerStorage($settings['installerDirectory'], $settings['installerBaseUrl']);
    }

    /** @return array<string, mixed> */
    public function listApps(): array
    {
        return ['apps' => $this->apps->all()];
    }

    /** @return array<string, mixed> */
    public function getApp(): array
    {
        return ['app' => $this->apps->get(requireInteger($_GET['id'] ?? null, 'ID aplikace', 1))];
    }

    /** @return array<string, mixed> */
    public function updateApp(): array
    {
        $request = readJsonBody();
        $id = $this->requireAppId($request);
        $changes = $this->requireAppData($request);

        $current = $this->apps->get($id);
        $url = (string) ($current['url'] ?? '');
        $installerType = $this->storage->installerTypeOfUrl($url);
        if ($installerType !== null) {
            $merged = array_merge(editableFieldsOf($current), $changes);
            $defaultFileName = defaultDownloadFileName($id, strtolower((string) ($merged['installerType'] ?? 'auto')), $url);
            assertInstallerTypeMatchesFile($merged, $installerType, $defaultFileName);
        }

        return ['message' => 'Změny aplikace byly uloženy.', 'app' => $this->apps->update($id, $changes)];
    }

    /** @return array<string, mixed> */
    public function enableApp(): array
    {
        return ['message' => 'Aplikace byla povolena.', 'app' => $this->apps->setEnabled($this->requireAppId(readJsonBody()), true)];
    }

    /** @return array<string, mixed> */
    public function disableApp(): array
    {
        return ['message' => 'Aplikace byla zakázána.', 'app' => $this->apps->setEnabled($this->requireAppId(readJsonBody()), false)];
    }

    /**
     * Deletes the app and its uploaded installer, unless another app uses the
     * same file. The file is only removed after the record is gone.
     *
     * @return array<string, mixed>
     */
    public function deleteApp(): array
    {
        $id = $this->requireAppId(readJsonBody());
        $app = $this->apps->get($id);
        $fileName = $this->storage->managedFileNameOf((string) ($app['url'] ?? ''));
        $otherApps = array_filter($this->apps->all(), function (array $other) use ($id): bool {
            return $other['id'] !== $id;
        });

        if ($fileName === null) {
            return $this->deletedResponse($id, 'Aplikace byla smazána. Její URL neodkazovala na soubor spravovaný touto administrací.');
        }
        if ($this->storage->isUsedByAnyOf($fileName, $otherApps)) {
            return $this->deletedResponse($id, 'Aplikace byla smazána. Instalační soubor zůstal zachován, protože jej používá jiný záznam.');
        }
        if (!pathExists($this->storage->pathOf($fileName))) {
            return $this->deletedResponse($id, 'Aplikace byla smazána. Instalační soubor už v nadřazené složce neexistoval.');
        }

        $quarantinePath = $this->storage->moveToQuarantine($fileName);
        try {
            $response = $this->deletedResponse($id, 'Aplikace i její instalační soubor byly smazány.');
        } catch (Throwable $exception) {
            $this->storage->restoreFromQuarantine($quarantinePath, $fileName);
            throw $exception;
        }

        if (@unlink($quarantinePath)) {
            return $response;
        }

        $response['warning'] = true;
        $response['message'] = $this->storage->restoreFromQuarantine($quarantinePath, $fileName)
            ? 'Aplikace byla smazána, ale instalační soubor se nepodařilo odstranit a byl vrácen na původní místo.'
            : 'Aplikace byla smazána, ale dočasný instalační soubor se nepodařilo odstranit. Zkontrolujte nadřazenou složku.';

        return $response;
    }

    /** @return array<string, mixed> */
    public function startUpload(): array
    {
        $request = readJsonBody();
        $fileName = requireNonEmptyText($request, 'fileName', 'Název instalačního souboru');
        $fileSize = requireInteger($request['fileSize'] ?? null, 'Velikost instalačního souboru', 1);
        $chunkSize = isset($request['chunkSizeBytes']) ? requireInteger($request['chunkSizeBytes'], 'Velikost nahrávacího bloku', 1) : null;

        return ['upload' => $this->chunkUpload()->start($fileName, $fileSize, $chunkSize)];
    }

    /** @return array<string, mixed> */
    public function receiveChunk(): array
    {
        $uploadId = (string) ($_GET['uploadId'] ?? '');
        $chunkIndex = requireInteger($_GET['chunkIndex'] ?? null, 'Pořadové číslo bloku', 0);
        $contentLength = ($_SERVER['CONTENT_LENGTH'] ?? '') !== ''
            ? requireInteger((string) $_SERVER['CONTENT_LENGTH'], 'Velikost těla požadavku', 0)
            : null;

        $input = fopen('php://input', 'rb');
        if ($input === false) {
            throw new RuntimeException('Data nahrávaného bloku se nepodařilo načíst.');
        }

        try {
            return ['chunk' => $this->chunkUpload()->receiveChunk($uploadId, $chunkIndex, $input, $contentLength)];
        } finally {
            fclose($input);
        }
    }

    /**
     * Finishes the upload and creates the (disabled) app for the installer.
     *
     * @return array<string, mixed>
     */
    public function finishUploadAndCreateApp(): array
    {
        $request = readJsonBody();
        $uploadId = requireNonEmptyText($request, 'uploadId', 'ID nahrávání');
        $appData = $this->requireAppData($request);

        $app = $this->chunkUpload()->finish($uploadId, function (string $fileName, string $extension) use ($appData): array {
            assertInstallerTypeMatchesFile($appData, $extension, $fileName);
            $appData['url'] = $this->storage->urlOf($fileName);
            if (trim((string) ($appData['fileName'] ?? '')) === '') {
                $appData['fileName'] = $fileName;
            }

            return $this->apps->create($appData);
        });

        return ['message' => 'Aplikace byla vytvořena a instalační soubor byl nahrán.', 'app' => $app];
    }

    /** @return array<string, mixed> */
    public function cancelUpload(): array
    {
        $cancelled = $this->chunkUpload()->cancel(requireNonEmptyText(readJsonBody(), 'uploadId', 'ID nahrávání'));

        return ['message' => $cancelled ? 'Nahrávání bylo zrušeno a dočasná data byla odstraněna.' : 'Nahrávací relace už neexistovala.'];
    }

    /** @return array<string, mixed> */
    private function deletedResponse(int $id, string $message): array
    {
        return ['warning' => false, 'message' => $message, 'app' => $this->apps->delete($id)];
    }

    private function chunkUpload(): ChunkUpload
    {
        return new ChunkUpload($this->settings, $this->storage, $this->sessionId);
    }

    /**
     * @param array<string, mixed> $request
     */
    private function requireAppId(array $request): int
    {
        return requireInteger($request['id'] ?? null, 'ID aplikace', 1);
    }

    /**
     * @param array<string, mixed> $request
     * @return array<string, mixed>
     */
    private function requireAppData(array $request): array
    {
        if (!is_array($request['app'] ?? null)) {
            throw new ApiError('Požadavek musí obsahovat objekt app.');
        }

        return $request['app'];
    }
}
