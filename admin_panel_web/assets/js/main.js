// The administration page: loads the app list and handles the buttons of the
// list and of both dialogs.
(function (WinFly, $, bootstrap) {
    'use strict';

    const api = WinFly.api;
    const ui = WinFly.ui;
    const form = WinFly.appForm;
    const table = WinFly.appTable;

    const state = {
        apps: [],
        loading: false,
        deleteId: null,
        upload: null,
        saving: false,
    };

    let appModal;
    let deleteModal;

    $(function () {
        appModal = bootstrap.Modal.getOrCreateInstance(document.getElementById('appModal'));
        deleteModal = bootstrap.Modal.getOrCreateInstance(document.getElementById('deleteModal'));
        form.bindEvents();
        bindEvents();
        loadApps();
    });

    function bindEvents() {
        $('#refreshButton').on('click', loadApps);
        $('#searchInput').on('input', renderApps);
        $(document).on('click', '[data-action]', handleActionButton);

        $('#appForm').on('submit', saveApp);
        $('#cancelAppButton').on('click', cancelAppDialog);
        $('#appModal')
            .on('hide.bs.modal', function (event) {
                if (state.saving) {
                    event.preventDefault();
                }
            })
            .on('shown.bs.modal', function () {
                $('#appName').trigger('focus');
            });

        $('#confirmDeleteButton').on('click', deleteApp);
        $(window).on('beforeunload', function () {
            return state.upload && state.upload.isActive() ? 'Probíhá nahrávání instalačního souboru.' : undefined;
        });
    }

    function handleActionButton() {
        const $button = $(this);
        const action = $button.data('action');
        const id = Number($button.data('id'));

        if (action === 'create') {
            form.openForCreate();
            appModal.show();
        } else if (action === 'reload') {
            loadApps();
        } else if (action === 'edit') {
            withApp(id, function (app) {
                form.openForEdit(app);
                appModal.show();
            });
        } else if (action === 'delete') {
            withApp(id, function (app) {
                state.deleteId = id;
                $('#deleteAppName').text(app.name || 'ID #' + id);
                deleteModal.show();
            });
        } else if (action === 'enable' || action === 'disable') {
            withApp(id, function () {
                setAppEnabled(id, action, $button);
            });
        }
    }

    /** Calls useApp with the app, or warns when it is no longer in the list. */
    function withApp(id, useApp) {
        const app = state.apps.find(function (candidate) {
            return candidate.id === id;
        });
        if (app) {
            useApp(app);
        } else {
            ui.showToast('Vybraná aplikace už není dostupná. Obnovte seznam.', 'warning');
        }
    }

    async function loadApps() {
        if (state.loading) {
            return;
        }

        state.loading = true;
        ui.setPageAlert('');
        $('#refreshButton').prop('disabled', true).text('Obnovuji…');
        table.renderLoading();

        try {
            const response = await api.get('list');
            state.apps = response.apps || [];
            renderApps();
        } catch (error) {
            const message = api.errorMessage(error, 'Seznam aplikací se nepodařilo načíst.');
            state.apps = [];
            table.renderLoadError(message);
            ui.setPageAlert(message, 'danger');
        } finally {
            state.loading = false;
            $('#refreshButton').prop('disabled', false).text('Obnovit');
        }
    }

    function renderApps() {
        table.renderApps(state.apps, String($('#searchInput').val()));
    }

    /** Puts a new or changed app into the list, sorted by ID. */
    function storeApp(app) {
        state.apps = state.apps.filter(function (current) {
            return current.id !== app.id;
        }).concat([app]).sort(function (left, right) {
            return left.id - right.id;
        });
        renderApps();
    }

    async function saveApp(event) {
        event.preventDefault();
        if (state.saving) {
            return;
        }

        form.showError('');
        if (!form.validate()) {
            return;
        }

        let appData;
        try {
            appData = form.read();
        } catch (error) {
            form.showError(error.message);
            return;
        }

        const id = form.editedAppId();
        state.saving = true;
        form.setBusy(true, id === null ? 'Zrušit nahrávání' : '');

        try {
            const response = id === null ? await uploadInstallerAndCreateApp(appData) : await api.post('update', {id: id, app: appData});
            storeApp(response.app);
            state.saving = false;
            appModal.hide();
            ui.showToast(response.message, 'success');
        } catch (error) {
            if (!error.cancelled) {
                form.showError(api.errorMessage(error, 'Aplikaci se nepodařilo uložit.'));
            }
        } finally {
            state.saving = false;
            state.upload = null;
            form.setBusy(false);
        }
    }

    function uploadInstallerAndCreateApp(appData) {
        state.upload = WinFly.createInstallerUpload(form.selectedFile(), function (progress) {
            form.showUploadProgress(progress);
            if (progress.finishing) {
                $('#cancelAppButton').prop('disabled', true).text('Dokončuji…');
            }
        });
        return state.upload.start(appData);
    }

    /** Closes the dialog; during an upload it first cancels the upload. */
    async function cancelAppDialog() {
        if (state.upload) {
            $('#cancelAppButton').prop('disabled', true).text('Ruším…');
            await state.upload.cancel();
        }
        if (!state.saving) {
            appModal.hide();
        }
    }

    async function deleteApp() {
        const $deleteModal = $('#deleteModal');
        ui.setButtonBusy($('#confirmDeleteButton'), true);
        $deleteModal.find('[data-bs-dismiss]').prop('disabled', true);

        try {
            const response = await api.post('delete', {id: state.deleteId});
            state.apps = state.apps.filter(function (app) {
                return app.id !== state.deleteId;
            });
            renderApps();
            deleteModal.hide();
            ui.showToast(response.message, response.warning ? 'warning' : 'success');
        } catch (error) {
            ui.showToast(api.errorMessage(error, 'Aplikaci se nepodařilo smazat.'), 'danger');
        } finally {
            ui.setButtonBusy($('#confirmDeleteButton'), false);
            $deleteModal.find('[data-bs-dismiss]').prop('disabled', false);
        }
    }

    async function setAppEnabled(id, action, $button) {
        ui.setInlineButtonBusy($button, true);
        try {
            const response = await api.post(action, {id: id});
            storeApp(response.app);
            ui.showToast(response.message, 'success');
        } catch (error) {
            ui.showToast(api.errorMessage(error, 'Stav aplikace se nepodařilo změnit.'), 'danger');
            ui.setInlineButtonBusy($button, false);
        }
    }
}(window.WinFly, window.jQuery, window.bootstrap));
