(function ($, bootstrap) {
    'use strict';

    const config = window.ProvisionerAdminConfig || {};
    const DEFAULT_CHUNK_SIZE = 1024 * 1024;
    const DEFAULT_MINIMUM_CHUNK_SIZE = 64 * 1024;
    const DEFAULT_PARALLEL_UPLOADS = 3;
    const DEFAULT_CHUNK_ATTEMPTS = 4;
    const DEFAULT_CHUNK_TIMEOUT_MS = 120000;

    const state = {
        apps: [],
        deleteId: null,
        loading: false,
        autoFileName: null,
        upload: null,
        allowAppModalClose: false,
    };

    let appModal;
    let deleteModal;

    $(initialize);

    function initialize() {
        appModal = bootstrap.Modal.getOrCreateInstance(document.getElementById('appModal'));
        deleteModal = bootstrap.Modal.getOrCreateInstance(document.getElementById('deleteModal'));

        prepareChunkUploadInterface();
        bindEvents();
        resetApplicationForm();
        loadApplications();
    }

    function prepareChunkUploadInterface() {
        const $cancelButton = $('#appModal .modal-footer [data-bs-dismiss="modal"]').first();
        $cancelButton
            .attr('id', 'cancelAppButton')
            .removeAttr('data-bs-dismiss')
            .text('Zrušit');

        const baseUrl = String(config.installerBaseUrl || '');
        const $note = $('#installerUploadGroup .upload-note');

        if ($note.length > 0) {
            $note.empty()
                .append('Povolené jsou pouze soubory ')
                .append($('<code>').text('.exe'))
                .append(' a ')
                .append($('<code>').text('.msi'))
                .append('. Instalátor se nahrává po malých blocích, takže celý soubor může být větší než limity ')
                .append($('<code>').text('post_max_size'))
                .append(' a ')
                .append($('<code>').text('upload_max_filesize'))
                .append('. Výsledná URL bude ')
                .append($('<code>').text(baseUrl.replace(/\/$/, '') + '/<název-souboru>'))
                .append('.');
        }

        if ($('#chunkUploadProgress').length === 0) {
            const $progress = $('<div>', {
                id: 'chunkUploadProgress',
                class: 'mt-3 d-none',
                'aria-live': 'polite',
            });

            const $header = $('<div>', {
                class: 'd-flex justify-content-between align-items-center gap-3 small mb-1',
            });
            $header.append(
                $('<span>', {id: 'chunkUploadStatus', text: 'Připravuji nahrávání…'}),
                $('<strong>', {id: 'chunkUploadPercent', text: '0 %'})
            );

            const $barContainer = $('<div>', {
                class: 'progress',
                role: 'progressbar',
                'aria-label': 'Průběh nahrávání instalátoru',
                'aria-valuemin': '0',
                'aria-valuemax': '100',
                'aria-valuenow': '0',
                style: 'height: 0.8rem;',
            });
            $barContainer.append($('<div>', {
                id: 'chunkUploadBar',
                class: 'progress-bar progress-bar-striped progress-bar-animated',
                style: 'width: 0%;',
            }));

            $progress.append(
                $header,
                $barContainer,
                $('<div>', {
                    id: 'chunkUploadDetails',
                    class: 'form-text mt-1',
                    text: '',
                })
            );

            $('#installerUploadGroup').append($progress);
        }
    }

    function bindEvents() {
        $('#createButton').on('click', openCreateModal);
        $('#refreshButton').on('click', loadApplications);
        $('#searchInput').on('input', renderApplications);

        $('#addArgumentButton').on('click', function () {
            addArgumentRow('');
        });

        $('#addExitCodeButton').on('click', function () {
            addExitCodeRow('');
        });

        $('#appForm').on('submit', saveApplication);
        $('#cancelAppButton').on('click', handleApplicationModalCancel);
        $('#confirmDeleteButton').on('click', deleteApplication);
        $('#installerFile').on('change', handleInstallerFileChange);
        $('#installerType').on('change', function () {
            validateInstallerSelection(isEditingForm());
        });

        $('#fileName').on('input', function () {
            const currentValue = String($(this).val() || '');
            if (state.autoFileName !== null && currentValue !== state.autoFileName) {
                state.autoFileName = null;
            }
        });

        $('#appsTableBody').on('click', '[data-action]', function () {
            const $button = $(this);
            const action = String($button.data('action'));

            if (action === 'create') {
                openCreateModal();
                return;
            }

            const id = Number($button.data('id'));
            if (!Number.isInteger(id) || id < 1) {
                showToast('Vybrané ID aplikace není platné.', 'danger');
                return;
            }

            if (action === 'edit') {
                openEditModal(id);
            } else if (action === 'delete') {
                openDeleteModal(id);
            } else if (action === 'enable' || action === 'disable') {
                changeEnabledState(id, action, $button);
            }
        });

        $('#argumentsList').on('click', '.remove-repeater-row', function () {
            $(this).closest('.repeater-row').remove();
            ensureArgumentInput();
        });

        $('#exitCodesList').on('click', '.remove-repeater-row', function () {
            $(this).closest('.repeater-row').remove();
            ensureExitCodeInput();
        });

        $('#appModal').on('show.bs.modal', function () {
            state.allowAppModalClose = false;
        });

        $('#appModal').on('hide.bs.modal', function (event) {
            if (isUploadActive() && !state.allowAppModalClose) {
                event.preventDefault();
            }
        });

        $('#appModal').on('shown.bs.modal', function () {
            $('#appName').trigger('focus');
        });

        $('#appModal').on('hidden.bs.modal', function () {
            state.allowAppModalClose = false;
            resetApplicationForm();
        });

        $('#deleteModal').on('hidden.bs.modal', function () {
            state.deleteId = null;
            $('#deleteAppName').text('');
            setButtonBusy($('#confirmDeleteButton'), false);
        });

        $(window).on('beforeunload', function () {
            if (isUploadActive()) {
                return 'Probíhá nahrávání instalačního souboru.';
            }
            return undefined;
        });
    }

    async function loadApplications() {
        if (state.loading) {
            return;
        }

        state.loading = true;
        hidePageAlert();
        setRefreshBusy(true);
        renderLoadingRow();

        try {
            const response = await apiRequest('list', 'GET');
            state.apps = Array.isArray(response.apps) ? response.apps : [];
            sortApplications();
            renderApplications();
        } catch (error) {
            const message = extractErrorMessage(error, 'Seznam aplikací se nepodařilo načíst.');
            state.apps = [];
            renderErrorRow(message);
            showPageAlert(message, 'danger');
        } finally {
            state.loading = false;
            setRefreshBusy(false);
        }
    }

    function renderApplications() {
        const $tbody = $('#appsTableBody');
        const query = String($('#searchInput').val() || '').trim().toLocaleLowerCase('cs-CZ');

        const filteredApps = state.apps.filter(function (app) {
            if (query === '') {
                return true;
            }

            const haystack = [
                app.id,
                app.name,
                app.description,
                app.url,
                app.fileName,
                app.installerType,
            ].join(' ').toLocaleLowerCase('cs-CZ');

            return haystack.includes(query);
        });

        $('#appCount').text(
            query === ''
                ? String(state.apps.length)
                : filteredApps.length + ' / ' + state.apps.length
        );

        if (filteredApps.length === 0) {
            renderEmptyRow(query !== '');
            return;
        }

        $tbody.html(filteredApps.map(renderApplicationRow).join(''));
    }

    function renderApplicationRow(app) {
        const enabled = app.enabled === true;
        const id = Number(app.id);
        const statusText = enabled ? 'Povoleno' : 'Zakázáno';
        const statusClass = enabled ? 'status-dot-enabled' : 'status-dot-disabled';
        const statusBadgeClass = enabled ? 'text-bg-success' : 'text-bg-secondary';
        const toggleAction = enabled ? 'disable' : 'enable';
        const toggleLabel = enabled ? 'Zakázat' : 'Povolit';
        const toggleClass = enabled ? 'btn-outline-warning' : 'btn-outline-success';
        const description = app.description
            ? formatMultilineText(app.description)
            : '<span class="text-body-secondary fst-italic">Bez popisu</span>';
        const installerTypeValue = String(app.installerType || 'auto').toLowerCase();
        const installerType = installerTypeValue === 'auto'
            ? 'AUTOMATICKY'
            : installerTypeValue.toUpperCase();
        const fileName = app.fileName || '';
        const url = String(app.url || '');
        const safeUrl = getSafeHttpUrl(url);

        return [
            '<tr data-app-id="', escapeHtml(String(id)), '">',
            '<td>',
            '<div class="d-flex align-items-start gap-2">',
            '<span class="status-dot ', statusClass, ' mt-2" aria-hidden="true"></span>',
            '<div class="min-w-0">',
            '<div class="d-flex flex-wrap align-items-center gap-2 mb-1">',
            '<strong class="app-name">', escapeHtml(String(app.name || '')), '</strong>',
            '<span class="badge ', statusBadgeClass, '">', statusText, '</span>',
            '</div>',
            '<div class="small text-body-secondary">',
            'ID #', escapeHtml(String(id)),
            ' &middot; ', escapeHtml(installerType),
            fileName
                ? ' &middot; <span class="app-file font-monospace">' + escapeHtml(String(fileName)) + '</span>'
                : '',
            '</div>',
            '</div>',
            '</div>',
            '</td>',
            '<td>',
            '<div class="app-description mb-1">', description, '</div>',
            safeUrl
                ? '<a class="app-url small font-monospace link-secondary" href="' +
                    escapeAttribute(safeUrl) +
                    '" target="_blank" rel="noopener noreferrer">' +
                    escapeHtml(url) +
                    '</a>'
                : url
                    ? '<span class="app-url small font-monospace text-danger">' + escapeHtml(url) + '</span>'
                    : '',
            '</td>',
            '<td class="text-end">',
            '<div class="action-buttons">',
            '<button type="button" class="btn btn-sm btn-outline-primary" data-action="edit" data-id="',
            escapeHtml(String(id)),
            '">Upravit</button>',
            '<button type="button" class="btn btn-sm ',
            toggleClass,
            '" data-action="',
            toggleAction,
            '" data-id="',
            escapeHtml(String(id)),
            '">',
            toggleLabel,
            '</button>',
            '<button type="button" class="btn btn-sm btn-outline-danger" data-action="delete" data-id="',
            escapeHtml(String(id)),
            '">Smazat</button>',
            '</div>',
            '</td>',
            '</tr>',
        ].join('');
    }

    function renderLoadingRow() {
        $('#appCount').text('…');
        $('#appsTableBody').html(
            '<tr><td colspan="3" class="text-center py-5 text-body-secondary">' +
            '<div class="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></div>' +
            'Načítám aplikace…</td></tr>'
        );
    }

    function renderEmptyRow(isSearchResult) {
        const heading = isSearchResult ? 'Žádná aplikace neodpovídá hledání' : 'Nejsou nastavené žádné aplikace';
        const text = isSearchResult
            ? 'Změňte hledaný výraz a zobrazte další záznamy.'
            : 'Přidejte první instalační záznam do apps.json.';
        const button = isSearchResult
            ? ''
            : '<button type="button" class="btn btn-primary mt-3" data-action="create">Přidat aplikaci</button>';

        $('#appsTableBody').html(
            '<tr><td colspan="3" class="text-center py-5">' +
            '<div class="empty-state">' +
            '<h3 class="h6 mb-2">' + escapeHtml(heading) + '</h3>' +
            '<p class="text-body-secondary mb-0">' + escapeHtml(text) + '</p>' +
            button +
            '</div></td></tr>'
        );
    }

    function renderErrorRow(message) {
        $('#appCount').text('0');
        $('#appsTableBody').html(
            '<tr><td colspan="3" class="text-center py-5">' +
            '<div class="empty-state">' +
            '<h3 class="h6 text-danger mb-2">Aplikace se nepodařilo načíst</h3>' +
            '<p class="text-body-secondary mb-3">' + escapeHtml(message) + '</p>' +
            '<button type="button" class="btn btn-outline-primary" id="retryLoadButton">Zkusit znovu</button>' +
            '</div></td></tr>'
        );

        $('#retryLoadButton').on('click', loadApplications);
    }

    function openCreateModal() {
        resetApplicationForm();
        configureCreateMode();
        $('#appModalTitle').text('Přidat aplikaci');
        $('#appModalSubtitle').text(
            'Nová aplikace dostane číselné ID a bude ve výchozím stavu zakázaná.'
        );
        $('#saveButton .button-label').text('Přidat aplikaci');
        appModal.show();
    }

    function openEditModal(id) {
        const app = findApplication(id);
        if (!app) {
            showToast('Vybraná aplikace už není dostupná. Obnovte seznam.', 'warning');
            return;
        }

        resetApplicationForm();
        configureEditMode();

        $('#appId').val(String(app.id));
        $('#appName').val(app.name || '');
        $('#appDescription').val(app.description || '');
        $('#sha256').val(app.sha256 || '');
        $('#fileName').val(app.fileName || '');
        $('#installerType').val(app.installerType || 'auto');
        $('#waitForInstaller').prop('checked', app.wait === true);

        setArgumentRows(Array.isArray(app.arguments) ? app.arguments : []);
        setExitCodeRows(Array.isArray(app.successExitCodes) ? app.successExitCodes : []);

        $('#appModalTitle').text('Upravit aplikaci');
        $('#appModalSubtitle').text(
            'Upravujete ID #' + app.id + '. URL a nahraný instalační soubor se při úpravě nemění.'
        );
        $('#saveButton .button-label').text('Uložit změny');
        appModal.show();
    }

    function openDeleteModal(id) {
        const app = findApplication(id);
        if (!app) {
            showToast('Vybraná aplikace už není dostupná. Obnovte seznam.', 'warning');
            return;
        }

        state.deleteId = id;
        $('#deleteAppName').text(app.name || ('ID #' + id));
        deleteModal.show();
    }

    async function saveApplication(event) {
        event.preventDefault();

        if (isUploadActive()) {
            return;
        }

        const $form = $('#appForm');
        const formElement = $form.get(0);
        const editing = isEditingForm();

        clearFormError();
        validateInstallerSelection(editing);
        $form.addClass('was-validated');

        if (!formElement.checkValidity()) {
            formElement.reportValidity();
            return;
        }

        let appData;
        try {
            appData = collectFormData();
        } catch (error) {
            showFormError(error instanceof Error ? error.message : String(error));
            return;
        }

        const rawId = String($('#appId').val() || '').trim();
        const id = editing ? Number(rawId) : null;

        if (editing && (!Number.isInteger(id) || id < 1)) {
            showFormError('ID aplikace není platné. Zavřete okno a obnovte stránku.');
            return;
        }

        setApplicationModalBusy(true, !editing);
        setButtonBusy($('#saveButton'), true);

        try {
            let response;

            if (editing) {
                response = await apiRequest('update', 'POST', {
                    id: id,
                    app: appData,
                });
            } else {
                const fileInput = document.getElementById('installerFile');
                const installer = fileInput.files[0];
                response = await createApplicationWithChunkedUpload(installer, appData);
            }

            upsertApplication(response.app);
            state.allowAppModalClose = true;
            appModal.hide();
            showToast(
                response.message || (editing ? 'Změny byly uloženy.' : 'Aplikace byla vytvořena.'),
                'success'
            );
        } catch (error) {
            if (!isUploadCancelledError(error)) {
                showFormError(extractErrorMessage(error, 'Aplikaci se nepodařilo uložit.'));
            }
        } finally {
            if (!isUploadActive()) {
                setApplicationModalBusy(false, false);
                setButtonBusy($('#saveButton'), false);
            }
        }
    }

    async function createApplicationWithChunkedUpload(file, appData) {
        let desiredChunkSize = DEFAULT_CHUNK_SIZE;
        let attemptNumber = 0;

        while (true) {
            attemptNumber += 1;
            const context = createUploadContext(file, desiredChunkSize);
            state.upload = context;
            showUploadProgress(context, attemptNumber > 1
                ? 'Opakuji nahrávání s menšími bloky…'
                : 'Připravuji blokové nahrávání…');

            try {
                const initResponse = await apiRequest('upload-init', 'POST', {
                    fileName: file.name,
                    fileSize: file.size,
                    chunkSizeBytes: desiredChunkSize,
                });

                const upload = initResponse && initResponse.upload ? initResponse.upload : {};
                context.uploadId = String(upload.uploadId || '');
                context.chunkSize = positiveIntegerOr(upload.chunkSizeBytes, desiredChunkSize);

                if (context.cancelled) {
                    throw createUploadCancelledError();
                }
                context.minimumChunkSize = positiveIntegerOr(
                    upload.minimumChunkSizeBytes,
                    DEFAULT_MINIMUM_CHUNK_SIZE
                );
                context.totalChunks = positiveIntegerOr(
                    upload.totalChunks,
                    Math.ceil(file.size / context.chunkSize)
                );
                context.startedAt = Date.now();
                context.phase = 'uploading';

                if (!/^[a-f0-9]{32}$/i.test(context.uploadId)) {
                    throw new Error('Server nevrátil platné ID blokového nahrávání.');
                }

                updateUploadProgress(context, 'Nahrávám instalační soubor…');
                await uploadAllChunks(context);

                if (context.cancelled) {
                    throw createUploadCancelledError();
                }

                context.phase = 'finalizing';
                $('#cancelAppButton').prop('disabled', true).text('Dokončuji…');
                setUploadProgressValues(100, 'Dokončuji nahrávání…',
                    formatBytes(file.size) + ' z ' + formatBytes(file.size) + ' · přesouvám soubor do cílové složky');

                const response = await apiRequest('upload-complete', 'POST', {
                    uploadId: context.uploadId,
                    app: appData,
                });

                context.active = false;
                if (state.upload === context) {
                    state.upload = null;
                }

                return response;
            } catch (error) {
                abortActiveChunkRequests(context);
                const cancelled = context.cancelled || isUploadCancelledError(error);

                if (context.uploadId !== '') {
                    await cancelUploadSession(context.uploadId);
                }

                context.active = false;
                if (state.upload === context) {
                    state.upload = null;
                }

                if (cancelled) {
                    throw createUploadCancelledError();
                }

                if (shouldReduceChunkSize(error, context)) {
                    const nextChunkSize = Math.max(
                        context.minimumChunkSize,
                        Math.floor(context.chunkSize / 2)
                    );

                    if (nextChunkSize < context.chunkSize) {
                        desiredChunkSize = nextChunkSize;
                        setUploadProgressValues(
                            0,
                            'Server odmítl velikost bloku, zkouším menší…',
                            'Nová velikost bloku: ' + formatBytes(desiredChunkSize)
                        );
                        await delay(600);
                        continue;
                    }
                }

                throw error;
            }
        }
    }

    function createUploadContext(file, requestedChunkSize) {
        return {
            active: true,
            cancelled: false,
            phase: 'initializing',
            uploadId: '',
            file: file,
            chunkSize: requestedChunkSize,
            minimumChunkSize: DEFAULT_MINIMUM_CHUNK_SIZE,
            totalChunks: Math.ceil(file.size / requestedChunkSize),
            completedBytes: 0,
            completedChunks: 0,
            loadedByChunk: new Map(),
            activeRequests: new Set(),
            startedAt: Date.now(),
            failed: false,
            primaryError: null,
        };
    }

    async function uploadAllChunks(context) {
        let nextChunkIndex = 0;
        const workerCount = Math.min(
            context.totalChunks,
            clampInteger(DEFAULT_PARALLEL_UPLOADS, 1, 6)
        );

        async function worker() {
            while (!context.failed && !context.cancelled) {
                const chunkIndex = nextChunkIndex;
                nextChunkIndex += 1;

                if (chunkIndex >= context.totalChunks) {
                    return;
                }

                try {
                    await uploadChunkWithRetry(context, chunkIndex);
                } catch (error) {
                    if (!context.primaryError && !isUploadCancelledError(error)) {
                        context.primaryError = error;
                    }
                    context.failed = true;
                    abortActiveChunkRequests(context);
                    throw error;
                }
            }

            if (context.cancelled) {
                throw createUploadCancelledError();
            }
        }

        const workers = [];
        for (let index = 0; index < workerCount; index += 1) {
            workers.push(worker());
        }

        try {
            await Promise.all(workers);
        } catch (error) {
            throw context.primaryError || error;
        }
    }

    async function uploadChunkWithRetry(context, chunkIndex) {
        const start = chunkIndex * context.chunkSize;
        const end = Math.min(start + context.chunkSize, context.file.size);
        const chunk = context.file.slice(start, end);
        const maximumAttempts = clampInteger(DEFAULT_CHUNK_ATTEMPTS, 1, 8);
        let lastError = null;

        for (let attempt = 1; attempt <= maximumAttempts; attempt += 1) {
            if (context.cancelled) {
                throw createUploadCancelledError();
            }

            try {
                await sendChunkRequest(context, chunkIndex, chunk);
                context.loadedByChunk.delete(chunkIndex);
                context.completedBytes += chunk.size;
                context.completedChunks += 1;
                updateUploadProgress(context, 'Nahrávám instalační soubor…');
                return;
            } catch (error) {
                context.loadedByChunk.delete(chunkIndex);
                updateUploadProgress(context, 'Nahrávání bloku se nezdařilo, opakuji…');

                if (context.cancelled || isUploadCancelledError(error)) {
                    throw createUploadCancelledError();
                }

                lastError = error;
                if (!isRetryableChunkError(error) || attempt >= maximumAttempts) {
                    break;
                }

                await cancellableDelay(Math.min(500 * (2 ** (attempt - 1)), 4000), context);
            }
        }

        const finalError = lastError || new Error('Blok instalačního souboru se nepodařilo nahrát.');
        try {
            finalError.provisionerUploadPhase = 'chunk';
            finalError.provisionerChunkSize = context.chunkSize;
        } catch (ignore) {
            // Některé hostitelské objekty nemusí být rozšiřitelné.
        }
        throw finalError;
    }

    function sendChunkRequest(context, chunkIndex, chunk) {
        return new Promise(function (resolve, reject) {
            const url = String(config.apiUrl || 'api.php')
                + '?action=upload-chunk'
                + '&uploadId=' + encodeURIComponent(context.uploadId)
                + '&chunkIndex=' + encodeURIComponent(String(chunkIndex));

            let request;
            request = $.ajax({
                url: url,
                method: 'POST',
                dataType: 'json',
                cache: false,
                timeout: DEFAULT_CHUNK_TIMEOUT_MS,
                headers: {
                    'X-CSRF-Token': String(config.csrfToken || ''),
                },
                data: chunk,
                contentType: 'application/octet-stream',
                processData: false,
                xhr: function () {
                    const xhr = $.ajaxSettings.xhr();
                    if (xhr.upload) {
                        xhr.upload.addEventListener('progress', function (event) {
                            if (!event.lengthComputable || context.cancelled) {
                                return;
                            }
                            context.loadedByChunk.set(chunkIndex, Math.min(event.loaded, chunk.size));
                            updateUploadProgress(context, 'Nahrávám instalační soubor…');
                        });
                    }
                    return xhr;
                },
            });

            context.activeRequests.add(request);

            request.done(function (response) {
                resolve(response);
            });

            request.fail(function (xhr, textStatus, errorThrown) {
                if (xhr && typeof xhr === 'object') {
                    xhr.provisionerTextStatus = textStatus;
                    xhr.provisionerErrorThrown = errorThrown;
                }

                if (xhr && Number(xhr.status) === 401) {
                    window.location.href = String(config.loginUrl || 'index.php');
                }

                reject(xhr || new Error('Požadavek na nahrání bloku selhal.'));
            });

            request.always(function () {
                context.activeRequests.delete(request);
            });
        });
    }

    function abortActiveChunkRequests(context) {
        context.activeRequests.forEach(function (request) {
            try {
                request.abort();
            } catch (ignore) {
                // Požadavek už mohl být dokončen.
            }
        });
        context.activeRequests.clear();
        context.loadedByChunk.clear();
    }

    async function handleApplicationModalCancel() {
        if (!isUploadActive()) {
            state.allowAppModalClose = true;
            appModal.hide();
            return;
        }

        const context = state.upload;
        context.cancelled = true;
        context.phase = 'cancelling';
        abortActiveChunkRequests(context);
        $('#cancelAppButton').prop('disabled', true).text('Ruším…');
        setUploadProgressValues(
            currentUploadPercent(context),
            'Ruším nahrávání…',
            'Odstraňuji dočasná data ze serveru.'
        );

        if (context.uploadId !== '') {
            await cancelUploadSession(context.uploadId);
        }

        context.active = false;
        if (state.upload === context) {
            state.upload = null;
        }

        state.allowAppModalClose = true;
        appModal.hide();
    }

    async function cancelUploadSession(uploadId) {
        try {
            await apiRequest('upload-cancel', 'POST', {uploadId: uploadId});
        } catch (error) {
            // Dočasné relace se na serveru automaticky čistí. Chyba zrušení
            // proto nemá přepsat původní chybu nahrávání.
        }
    }

    function shouldReduceChunkSize(error, context) {
        if (
            !error
            || error.provisionerUploadPhase !== 'chunk'
            || context.chunkSize <= context.minimumChunkSize
        ) {
            return false;
        }

        const status = Number(error.status || 0);
        const code = getApiErrorCode(error);
        const textStatus = String(error.provisionerTextStatus || '');

        if (code === 'CHUNK_BODY_SIZE') {
            return true;
        }

        if ([408, 413, 429, 502, 503, 504].includes(status)) {
            return true;
        }

        if (status === 0 || textStatus === 'timeout') {
            return true;
        }

        return status >= 500 && (!error.responseJSON || typeof error.responseJSON !== 'object');
    }

    function isRetryableChunkError(error) {
        if (!error) {
            return true;
        }

        const status = Number(error.status || 0);
        const code = getApiErrorCode(error);
        const textStatus = String(error.provisionerTextStatus || '');

        if (code === 'CHUNK_BODY_SIZE' || status === 413) {
            return false;
        }

        if (status === 401 || status === 403 || status === 404 || status === 409 || status === 422) {
            return false;
        }

        return status === 0
            || status === 408
            || status === 429
            || status >= 500
            || textStatus === 'timeout';
    }

    function getApiErrorCode(error) {
        return error
            && error.responseJSON
            && typeof error.responseJSON.code === 'string'
            ? error.responseJSON.code
            : '';
    }

    function isUploadActive() {
        return Boolean(state.upload && state.upload.active === true);
    }

    function createUploadCancelledError() {
        const error = new Error('Nahrávání bylo zrušeno.');
        error.provisionerUploadCancelled = true;
        return error;
    }

    function isUploadCancelledError(error) {
        return Boolean(
            error
            && (
                error.provisionerUploadCancelled === true
                || error.provisionerTextStatus === 'abort'
            )
        );
    }

    function showUploadProgress(context, status) {
        $('#chunkUploadProgress').removeClass('d-none');
        setUploadProgressValues(0, status, 'Velikost souboru: ' + formatBytes(context.file.size));
    }

    function hideUploadProgress() {
        $('#chunkUploadProgress').addClass('d-none');
        setUploadProgressValues(0, 'Připravuji nahrávání…', '');
    }

    function updateUploadProgress(context, status) {
        const loadedBytes = currentUploadedBytes(context);
        const percent = context.file.size > 0
            ? Math.min(100, (loadedBytes / context.file.size) * 100)
            : 0;
        const elapsedSeconds = Math.max(0.001, (Date.now() - context.startedAt) / 1000);
        const speed = loadedBytes / elapsedSeconds;
        const details = [
            formatBytes(loadedBytes) + ' z ' + formatBytes(context.file.size),
            formatBytes(speed) + '/s',
            'blok ' + formatBytes(context.chunkSize),
            context.completedChunks + ' / ' + context.totalChunks,
        ].join(' · ');

        setUploadProgressValues(percent, status, details);
    }

    function setUploadProgressValues(percent, status, details) {
        const safePercent = Math.max(0, Math.min(100, Number(percent) || 0));
        const rounded = Math.floor(safePercent);

        $('#chunkUploadStatus').text(status);
        $('#chunkUploadPercent').text(rounded + ' %');
        $('#chunkUploadDetails').text(details);
        $('#chunkUploadBar').css('width', safePercent.toFixed(2) + '%');
        $('#chunkUploadProgress .progress').attr('aria-valuenow', String(rounded));
    }

    function currentUploadedBytes(context) {
        let loaded = context.completedBytes;
        context.loadedByChunk.forEach(function (value) {
            loaded += value;
        });
        return Math.min(context.file.size, loaded);
    }

    function currentUploadPercent(context) {
        return context.file.size > 0
            ? (currentUploadedBytes(context) / context.file.size) * 100
            : 0;
    }

    function formatBytes(value) {
        const bytes = Math.max(0, Number(value) || 0);
        const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
        let unitIndex = 0;
        let size = bytes;

        while (size >= 1024 && unitIndex < units.length - 1) {
            size /= 1024;
            unitIndex += 1;
        }

        const decimals = unitIndex === 0 || size >= 100 ? 0 : size >= 10 ? 1 : 2;
        return size.toLocaleString('cs-CZ', {
            minimumFractionDigits: 0,
            maximumFractionDigits: decimals,
        }) + ' ' + units[unitIndex];
    }

    function positiveIntegerOr(value, fallback) {
        const parsed = Number(value);
        return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : fallback;
    }

    function clampInteger(value, minimum, maximum) {
        const parsed = Number(value);
        if (!Number.isFinite(parsed)) {
            return minimum;
        }
        return Math.max(minimum, Math.min(maximum, Math.floor(parsed)));
    }

    function delay(milliseconds) {
        return new Promise(function (resolve) {
            window.setTimeout(resolve, milliseconds);
        });
    }

    function cancellableDelay(milliseconds, context) {
        return new Promise(function (resolve, reject) {
            const startedAt = Date.now();

            function tick() {
                if (context.cancelled) {
                    reject(createUploadCancelledError());
                    return;
                }

                const remaining = milliseconds - (Date.now() - startedAt);
                if (remaining <= 0) {
                    resolve();
                    return;
                }

                window.setTimeout(tick, Math.min(remaining, 100));
            }

            tick();
        });
    }

    async function deleteApplication() {
        const id = state.deleteId;
        if (!Number.isInteger(id) || id < 1) {
            showToast('Vybrané ID aplikace není platné.', 'danger');
            deleteModal.hide();
            return;
        }

        setButtonBusy($('#confirmDeleteButton'), true);
        setModalControlsDisabled($('#deleteModal'), true);

        try {
            const response = await apiRequest('delete', 'POST', {id: id});

            state.apps = state.apps.filter(function (app) {
                return Number(app.id) !== id;
            });

            renderApplications();
            deleteModal.hide();
            showToast(
                response.message || 'Aplikace byla smazána.',
                response.warning === true ? 'warning' : 'success'
            );
        } catch (error) {
            showToast(extractErrorMessage(error, 'Aplikaci se nepodařilo smazat.'), 'danger');
        } finally {
            setButtonBusy($('#confirmDeleteButton'), false);
            setModalControlsDisabled($('#deleteModal'), false);
        }
    }

    async function changeEnabledState(id, action, $button) {
        const app = findApplication(id);
        if (!app) {
            showToast('Vybraná aplikace už není dostupná. Obnovte seznam.', 'warning');
            return;
        }

        setInlineButtonBusy($button, true);

        try {
            const response = await apiRequest(action, 'POST', {id: id});
            upsertApplication(response.app);
            showToast(
                response.message || (action === 'enable' ? 'Aplikace byla povolena.' : 'Aplikace byla zakázána.'),
                'success'
            );
        } catch (error) {
            showToast(extractErrorMessage(error, 'Stav aplikace se nepodařilo změnit.'), 'danger');
            setInlineButtonBusy($button, false);
        }
    }

    function collectFormData() {
        const argumentsList = [];

        $('#argumentsList .argument-value').each(function () {
            const value = String($(this).val() || '').trim();
            if (value !== '') {
                argumentsList.push(value);
            }
        });

        const successExitCodes = [];

        $('#exitCodesList .exit-code-value').each(function () {
            const rawValue = String($(this).val() || '').trim();
            if (rawValue === '') {
                return;
            }

            if (!/^-?\d+$/.test(rawValue)) {
                throw new Error('Každý úspěšný návratový kód musí být celé číslo.');
            }

            const value = Number(rawValue);
            if (!Number.isSafeInteger(value)) {
                throw new Error('Návratový kód je mimo bezpečný celočíselný rozsah JavaScriptu.');
            }

            if (!successExitCodes.includes(value)) {
                successExitCodes.push(value);
            }
        });

        return {
            name: String($('#appName').val() || '').trim(),
            description: String($('#appDescription').val() || '').trim(),
            sha256: String($('#sha256').val() || '').trim(),
            fileName: String($('#fileName').val() || '').trim(),
            installerType: String($('#installerType').val() || 'auto'),
            arguments: argumentsList,
            wait: $('#waitForInstaller').is(':checked'),
            successExitCodes: successExitCodes,
        };
    }

    function handleInstallerFileChange() {
        const input = document.getElementById('installerFile');
        const file = input.files && input.files.length > 0 ? input.files[0] : null;

        if (file) {
            const currentFileName = String($('#fileName').val() || '');

            if (currentFileName.trim() === '' || currentFileName === state.autoFileName) {
                $('#fileName').val(file.name);
                state.autoFileName = file.name;
            }
        }

        hideUploadProgress();
        validateInstallerSelection(false);
    }

    function validateInstallerSelection(editing) {
        const fileInput = document.getElementById('installerFile');
        const typeInput = document.getElementById('installerType');

        fileInput.setCustomValidity('');
        typeInput.setCustomValidity('');

        if (editing || fileInput.disabled) {
            return;
        }

        const file = fileInput.files && fileInput.files.length > 0
            ? fileInput.files[0]
            : null;

        if (!file) {
            fileInput.setCustomValidity('Vyberte instalační soubor EXE nebo MSI.');
            return;
        }

        const extension = getFileExtension(file.name);
        if (extension !== 'exe' && extension !== 'msi') {
            fileInput.setCustomValidity('Nahrát lze pouze instalační soubor EXE nebo MSI.');
            return;
        }

        const installerType = String(typeInput.value || 'auto').toLowerCase();
        if (installerType !== 'auto' && installerType !== extension) {
            typeInput.setCustomValidity(
                'Zvolený typ instalátoru neodpovídá příponě nahraného souboru.'
            );
        }
    }

    function getFileExtension(fileName) {
        const match = /\.([^.]+)$/.exec(String(fileName || ''));
        return match ? String(match[1]).toLowerCase() : '';
    }

    function isEditingForm() {
        return String($('#appId').val() || '').trim() !== '';
    }

    function configureCreateMode() {
        const fileInput = document.getElementById('installerFile');

        $('#installerUploadGroup').removeClass('d-none');
        fileInput.disabled = false;
        fileInput.required = true;
        fileInput.setCustomValidity('');
    }

    function configureEditMode() {
        const fileInput = document.getElementById('installerFile');

        $('#installerUploadGroup').addClass('d-none');
        fileInput.required = false;
        fileInput.disabled = true;
        fileInput.setCustomValidity('');
        state.autoFileName = null;
    }

    function resetApplicationForm() {
        const $form = $('#appForm');
        const formElement = $form.get(0);

        if (isUploadActive()) {
            return;
        }

        formElement.reset();
        $form.removeClass('was-validated');
        $('#appId').val('');
        $('#installerType').val('auto');
        $('#waitForInstaller').prop('checked', true);
        $('#argumentsList').empty();
        $('#exitCodesList').empty();

        addArgumentRow('');
        [0, 1641, 3010].forEach(addExitCodeRow);

        state.autoFileName = null;
        state.upload = null;
        clearFormError();
        hideUploadProgress();
        setApplicationModalBusy(false, false);
        setButtonBusy($('#saveButton'), false);
        configureCreateMode();

        document.getElementById('installerType').setCustomValidity('');
    }

    function setArgumentRows(values) {
        $('#argumentsList').empty();

        values.forEach(function (value) {
            addArgumentRow(String(value));
        });

        ensureArgumentInput();
    }

    function setExitCodeRows(values) {
        $('#exitCodesList').empty();

        values.forEach(function (value) {
            addExitCodeRow(String(value));
        });

        ensureExitCodeInput();
    }

    function addArgumentRow(value) {
        const $row = $('<div>', {class: 'repeater-row'});
        const $input = $('<input>', {
            type: 'text',
            class: 'form-control form-control-sm font-monospace argument-value',
            placeholder: '/quiet',
            'aria-label': 'Argument instalátoru',
        }).val(value);
        const $remove = createRemoveButton('Odebrat argument');

        $row.append($input, $remove);
        $('#argumentsList').append($row);
    }

    function addExitCodeRow(value) {
        const $row = $('<div>', {class: 'repeater-row'});
        const $input = $('<input>', {
            type: 'number',
            step: '1',
            class: 'form-control form-control-sm font-monospace exit-code-value',
            placeholder: '0',
            'aria-label': 'Úspěšný návratový kód',
        }).val(value);
        const $remove = createRemoveButton('Odebrat návratový kód');

        $row.append($input, $remove);
        $('#exitCodesList').append($row);
    }

    function createRemoveButton(label) {
        return $('<button>', {
            type: 'button',
            class: 'btn btn-sm btn-outline-secondary remove-repeater-row',
            text: 'Odebrat',
            'aria-label': label,
        });
    }

    function ensureArgumentInput() {
        if ($('#argumentsList .repeater-row').length === 0) {
            addArgumentRow('');
        }
    }

    function ensureExitCodeInput() {
        if ($('#exitCodesList .repeater-row').length === 0) {
            addExitCodeRow('');
        }
    }

    function upsertApplication(app) {
        if (!app || !Number.isInteger(Number(app.id))) {
            return;
        }

        const id = Number(app.id);
        const index = state.apps.findIndex(function (current) {
            return Number(current.id) === id;
        });

        if (index === -1) {
            state.apps.push(app);
        } else {
            state.apps[index] = app;
        }

        sortApplications();
        renderApplications();
    }

    function sortApplications() {
        state.apps.sort(function (left, right) {
            return Number(left.id) - Number(right.id);
        });
    }

    function findApplication(id) {
        return state.apps.find(function (app) {
            return Number(app.id) === Number(id);
        }) || null;
    }

    function apiRequest(action, method, payload) {
        const options = {
            url: String(config.apiUrl || 'api.php') + '?action=' + encodeURIComponent(action),
            method: method,
            dataType: 'json',
            cache: false,
            headers: {
                'X-CSRF-Token': String(config.csrfToken || ''),
            },
        };

        if (payload !== undefined) {
            options.contentType = 'application/json; charset=utf-8';
            options.data = JSON.stringify(payload);
            options.processData = false;
        }

        return redirectOnUnauthorized($.ajax(options));
    }

    function redirectOnUnauthorized(request) {
        request.fail(function (xhr) {
            if (xhr && Number(xhr.status) === 401) {
                window.location.href = String(config.loginUrl || 'index.php');
            }
        });

        return request;
    }

    function extractErrorMessage(error, fallback) {
        if (error && error.responseJSON && typeof error.responseJSON.message === 'string') {
            return error.responseJSON.message;
        }

        if (error && typeof error.responseText === 'string' && error.responseText.trim() !== '') {
            try {
                const parsed = JSON.parse(error.responseText);
                if (parsed && typeof parsed.message === 'string') {
                    return parsed.message;
                }
            } catch (parseError) {
                // Odpověď nebyla JSON; použije se obecná zpráva.
            }
        }

        if (error instanceof Error && error.message) {
            return error.message;
        }

        return fallback;
    }

    function showFormError(message) {
        $('#formError').removeClass('d-none').text(message);
    }

    function clearFormError() {
        $('#formError').addClass('d-none').text('');
    }

    function showPageAlert(message, type) {
        $('#pageAlert')
            .removeClass('d-none alert-success alert-danger alert-warning alert-info')
            .addClass('alert-' + type)
            .text(message);
    }

    function hidePageAlert() {
        $('#pageAlert')
            .addClass('d-none')
            .removeClass('alert-success alert-danger alert-warning alert-info')
            .text('');
    }

    function showToast(message, type) {
        const toastClass = type === 'danger'
            ? 'text-bg-danger'
            : type === 'warning'
                ? 'text-bg-warning'
                : 'text-bg-success';
        const closeClass = type === 'warning'
            ? 'btn-close me-2 m-auto'
            : 'btn-close btn-close-white me-2 m-auto';

        const $toast = $('<div>', {
            class: 'toast align-items-center border-0 ' + toastClass,
            role: 'status',
            'aria-live': 'polite',
            'aria-atomic': 'true',
        });
        const $layout = $('<div>', {class: 'd-flex'});
        const $body = $('<div>', {class: 'toast-body'}).text(message);
        const $close = $('<button>', {
            type: 'button',
            class: closeClass,
            'data-bs-dismiss': 'toast',
            'aria-label': 'Zavřít',
        });

        $layout.append($body, $close);
        $toast.append($layout);
        $('#toastContainer').append($toast);

        const instance = new bootstrap.Toast($toast.get(0), {delay: 5500});

        $toast.on('hidden.bs.toast', function () {
            instance.dispose();
            $toast.remove();
        });

        instance.show();
    }

    function setButtonBusy($button, busy) {
        $button.prop('disabled', busy);
        $button.find('.spinner-border').toggleClass('d-none', !busy);
    }

    function setInlineButtonBusy($button, busy) {
        if (busy) {
            if (!$button.data('original-html')) {
                $button.data('original-html', $button.html());
            }

            $button
                .prop('disabled', true)
                .html('<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span>');
            return;
        }

        const originalHtml = $button.data('original-html');
        if (originalHtml) {
            $button.html(originalHtml).removeData('original-html');
        }

        $button.prop('disabled', false);
    }

    function setApplicationModalBusy(busy, allowUploadCancel) {
        const $controls = $('#appForm')
            .find('input, textarea, select, button')
            .not('#saveButton, #cancelAppButton');
        const $closeButtons = $('#appModal .btn-close');

        setStoredDisabledState($controls, busy);
        setStoredDisabledState($closeButtons, busy);

        const $cancelButton = $('#cancelAppButton');
        if (busy) {
            $cancelButton
                .prop('disabled', !allowUploadCancel)
                .text(allowUploadCancel ? 'Zrušit nahrávání' : 'Čekejte…');
        } else {
            $cancelButton.prop('disabled', false).text('Zrušit');
        }
    }

    function setStoredDisabledState($elements, disabled) {
        $elements.each(function () {
            const $element = $(this);

            if (disabled) {
                if ($element.data('provisioner-disabled-before-busy') === undefined) {
                    $element.data('provisioner-disabled-before-busy', this.disabled === true);
                }
                this.disabled = true;
                return;
            }

            const previous = $element.data('provisioner-disabled-before-busy');
            if (previous !== undefined) {
                this.disabled = previous === true;
                $element.removeData('provisioner-disabled-before-busy');
            }
        });
    }

    function setModalControlsDisabled($modal, disabled) {
        $modal.find('[data-bs-dismiss="modal"]').prop('disabled', disabled);
    }

    function setRefreshBusy(busy) {
        const $button = $('#refreshButton');
        $button.prop('disabled', busy);
        $button.text(busy ? 'Obnovuji…' : 'Obnovit');
    }

    function formatMultilineText(value) {
        return escapeHtml(String(value)).replace(/\r?\n/g, '<br>');
    }

    function getSafeHttpUrl(value) {
        try {
            const parsed = new URL(String(value));
            return parsed.protocol === 'http:' || parsed.protocol === 'https:'
                ? parsed.href
                : '';
        } catch (error) {
            return '';
        }
    }

    function escapeAttribute(value) {
        return escapeHtml(value);
    }

    function escapeHtml(value) {
        return String(value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }
}(window.jQuery, window.bootstrap));
