// Uploads an installer to api.php in chunks (several in parallel, each retried
// a few times) and then creates the app. When the server rejects the chunk
// size, the whole upload starts again with half as big chunks.
(function (WinFly) {
    'use strict';

    const api = WinFly.api;
    const helpers = WinFly.helpers;

    const FIRST_CHUNK_SIZE = 1024 * 1024;
    const PARALLEL_CHUNKS = 3;
    const ATTEMPTS_PER_CHUNK = 4;
    const CHUNK_TIMEOUT_MS = 120000;
    const STATUS_UPLOADING = 'Nahrávám instalační soubor…';

    /**
     * Returns {start(appData), cancel(), isActive()}. onProgress receives
     * {percent, status, details, finishing}; finishing is true while the server
     * moves the file into place, when the upload can no longer be cancelled.
     */
    function createInstallerUpload(file, onProgress) {
        const requests = new Set();
        let active = false;
        let cancelled = false;
        let running = Promise.resolve();
        let rejectWaits;
        const cancelSignal = new Promise(function (resolve, reject) {
            rejectWaits = reject;
        });
        cancelSignal.catch(function () {});

        function start(appData) {
            active = true;
            running = uploadAndCreateApp(appData).finally(function () {
                active = false;
            });
            return running;
        }

        /** Stops the upload and resolves once the server removed the temporary data. */
        async function cancel() {
            cancelled = true;
            rejectWaits(cancelledError());
            abortRequests();
            await running.catch(function () {});
        }

        async function uploadAndCreateApp(appData) {
            let chunkSize = FIRST_CHUNK_SIZE;

            for (;;) {
                let upload = null;
                let sendingChunks = false;

                try {
                    report(0, chunkSize === FIRST_CHUNK_SIZE ? 'Připravuji nahrávání…' : 'Opakuji nahrávání s menšími bloky…',
                        'Velikost souboru: ' + helpers.formatBytes(file.size));

                    const response = await track(api.post('upload-init', {
                        fileName: file.name,
                        fileSize: file.size,
                        chunkSizeBytes: chunkSize,
                    }));
                    upload = createProgress(response.upload);
                    throwIfCancelled();

                    sendingChunks = true;
                    await sendAllChunks(upload);
                    sendingChunks = false;
                    throwIfCancelled();

                    report(100, 'Dokončuji nahrávání…', helpers.formatBytes(file.size) + ' · přesouvám soubor do cílové složky', true);
                    return await api.post('upload-complete', {uploadId: upload.uploadId, app: appData});
                } catch (error) {
                    abortRequests();
                    if (upload) {
                        upload.stopped = true;
                        await api.post('upload-cancel', {uploadId: upload.uploadId}).catch(function () {
                            // The server removes forgotten uploads itself later.
                        });
                    }
                    if (cancelled || error.aborted) {
                        throw cancelledError();
                    }

                    const smallerChunkSize = sendingChunks ? smallerChunkSizeAfter(error, upload) : 0;
                    if (smallerChunkSize === 0) {
                        throw error;
                    }
                    chunkSize = smallerChunkSize;
                    report(0, 'Server odmítl velikost bloku, zkouším menší…', 'Nová velikost bloku: ' + helpers.formatBytes(chunkSize));
                    await helpers.delay(600);
                }
            }
        }

        function createProgress(serverUpload) {
            if (!serverUpload || !/^[a-f0-9]{32}$/i.test(serverUpload.uploadId)) {
                throw new Error('Server nevrátil platné ID nahrávání.');
            }

            return {
                uploadId: serverUpload.uploadId,
                chunkSize: serverUpload.chunkSizeBytes,
                minimumChunkSize: serverUpload.minimumChunkSizeBytes,
                totalChunks: serverUpload.totalChunks,
                nextChunk: 0,
                doneChunks: 0,
                doneBytes: 0,
                bytesInFlight: new Map(),
                startedAt: Date.now(),
                stopped: false,
            };
        }

        async function sendAllChunks(upload) {
            async function sendChunksInTurn() {
                while (!upload.stopped && upload.nextChunk < upload.totalChunks) {
                    const chunkIndex = upload.nextChunk;
                    upload.nextChunk += 1;
                    await sendChunkWithRetries(upload, chunkIndex);
                }
            }

            const senders = [];
            for (let i = 0; i < Math.min(PARALLEL_CHUNKS, upload.totalChunks); i += 1) {
                senders.push(sendChunksInTurn());
            }

            try {
                await Promise.all(senders);
            } catch (error) {
                upload.stopped = true;
                throw error;
            }
        }

        async function sendChunkWithRetries(upload, chunkIndex) {
            const offset = chunkIndex * upload.chunkSize;
            const chunk = file.slice(offset, offset + upload.chunkSize);

            for (let attempt = 1; ; attempt += 1) {
                try {
                    await track(api.request('upload-chunk', {
                        query: {uploadId: upload.uploadId, chunkIndex: chunkIndex},
                        body: chunk,
                        timeout: CHUNK_TIMEOUT_MS,
                        onUploadProgress: function (event) {
                            if (event.lengthComputable) {
                                upload.bytesInFlight.set(chunkIndex, Math.min(event.loaded, chunk.size));
                                reportUploadProgress(upload, STATUS_UPLOADING);
                            }
                        },
                    }));
                    upload.bytesInFlight.delete(chunkIndex);
                    upload.doneBytes += chunk.size;
                    upload.doneChunks += 1;
                    reportUploadProgress(upload, STATUS_UPLOADING);
                    return;
                } catch (error) {
                    upload.bytesInFlight.delete(chunkIndex);
                    if (upload.stopped || error.aborted || attempt >= ATTEMPTS_PER_CHUNK || !isWorthRetrying(error)) {
                        throw error;
                    }
                    reportUploadProgress(upload, 'Nahrávání bloku se nezdařilo, opakuji…');
                    await Promise.race([helpers.delay(Math.min(500 * 2 ** (attempt - 1), 4000)), cancelSignal]);
                }
            }
        }

        function reportUploadProgress(upload, status) {
            let uploadedBytes = upload.doneBytes;
            upload.bytesInFlight.forEach(function (bytes) {
                uploadedBytes += bytes;
            });
            uploadedBytes = Math.min(file.size, uploadedBytes);

            const seconds = Math.max(0.001, (Date.now() - upload.startedAt) / 1000);
            report(uploadedBytes / file.size * 100, status, [
                helpers.formatBytes(uploadedBytes) + ' z ' + helpers.formatBytes(file.size),
                helpers.formatBytes(uploadedBytes / seconds) + '/s',
                'blok ' + helpers.formatBytes(upload.chunkSize),
                upload.doneChunks + ' / ' + upload.totalChunks,
            ].join(' · '));
        }

        function report(percent, status, details, finishing) {
            onProgress({percent: percent, status: status, details: details, finishing: finishing === true});
        }

        /** Remembers a running request, so cancel() can abort it. */
        function track(request) {
            requests.add(request);
            return request.finally(function () {
                requests.delete(request);
            });
        }

        function abortRequests() {
            requests.forEach(function (request) {
                request.abort();
            });
            requests.clear();
        }

        function throwIfCancelled() {
            if (cancelled) {
                throw cancelledError();
            }
        }

        return {
            start: start,
            cancel: cancel,
            isActive: function () {
                return active;
            },
        };
    }

    /** Server errors and timeouts may pass when the chunk is sent again. */
    function isWorthRetrying(error) {
        if (error.code === 'CHUNK_BODY_SIZE' || [401, 403, 404, 409, 413, 422].includes(error.status)) {
            return false;
        }
        return error.status === 0 || error.status === 408 || error.status === 429 || error.status >= 500 || error.timedOut;
    }

    /**
     * Returns a smaller chunk size when the error looks like the server or a
     * proxy refused the request size, otherwise 0.
     */
    function smallerChunkSizeAfter(error, upload) {
        const looksLikeSizeLimit = error.code === 'CHUNK_BODY_SIZE'
            || [0, 408, 413, 429, 502, 503, 504].includes(error.status)
            || error.timedOut
            || (error.status >= 500 && !error.hasJsonBody);
        const smaller = Math.max(upload.minimumChunkSize, Math.floor(upload.chunkSize / 2));

        return looksLikeSizeLimit && smaller < upload.chunkSize ? smaller : 0;
    }

    function cancelledError() {
        const error = new Error('Nahrávání bylo zrušeno.');
        error.cancelled = true;
        return error;
    }

    WinFly.createInstallerUpload = createInstallerUpload;
}(window.WinFly));
