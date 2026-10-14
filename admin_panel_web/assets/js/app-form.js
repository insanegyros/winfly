// The "add / edit app" dialog: filling, reading and checking the form, the
// argument and exit code lists, and the upload progress bar.
(function (WinFly, $) {
    'use strict';

    const helpers = WinFly.helpers;
    const DEFAULT_EXIT_CODES = [0, 1641, 3010];

    // The fileName value filled in from the chosen file; replaced again when
    // another file is chosen, unless the administrator changed it.
    let autoFileName = null;

    function openForCreate() {
        reset();
        $('#appModalTitle').text('Přidat aplikaci');
        $('#appModalSubtitle').text('Nová aplikace dostane číselné ID a bude ve výchozím stavu zakázaná.');
        $('#saveButton .button-label').text('Přidat aplikaci');
        refreshInstallerRules();
    }

    function openForEdit(app) {
        reset();
        $('#installerUploadGroup').addClass('d-none');
        $('#installerFile').prop({disabled: true, required: false});
        $('#appForm').data('installerType', helpers.installerTypeFromUrl(app.url));

        $('#appId').val(app.id);
        $('#appName').val(app.name || '');
        $('#appDescription').val(app.description || '');
        $('#sha256').val(app.sha256 || '');
        $('#fileName').val(app.fileName || '');
        $('#installerType').val(app.installerType || 'auto');
        $('#waitForInstaller').prop('checked', app.wait === true);
        setRows('#argumentsList', app.arguments || []);
        setRows('#exitCodesList', app.successExitCodes || []);

        $('#appModalTitle').text('Upravit aplikaci');
        $('#appModalSubtitle').text('Upravujete ID #' + app.id + '. URL a nahraný instalační soubor se při úpravě nemění.');
        $('#saveButton .button-label').text('Uložit změny');
        refreshInstallerRules();
    }

    function reset() {
        $('#appForm').removeClass('was-validated').removeData('installerType')[0].reset();
        $('#appId').val('');
        $('#installerUploadGroup').removeClass('d-none');
        $('#installerFile').prop({disabled: false, required: true});
        setRows('#argumentsList', []);
        setRows('#exitCodesList', DEFAULT_EXIT_CODES);
        autoFileName = null;
        showError('');
        hideUploadProgress();
        setBusy(false);
    }

    /** The ID of the app being edited, or null when a new app is being added. */
    function editedAppId() {
        const id = $('#appId').val();
        return id === '' ? null : Number(id);
    }

    function selectedFile() {
        const files = $('#installerFile')[0].files;
        return files && files.length > 0 ? files[0] : null;
    }

    /** Checks the form, shows the problems and returns true when it can be saved. */
    function validate() {
        refreshInstallerRules();
        $('#appForm').addClass('was-validated');
        return $('#appForm')[0].reportValidity();
    }

    /** Returns the app fields the way api.php expects them. Throws on a bad exit code. */
    function read() {
        return {
            name: $('#appName').val().trim(),
            description: $('#appDescription').val().trim(),
            sha256: $('#sha256').val().trim(),
            fileName: $('#fileName').val().trim(),
            installerType: $('#installerType').val(),
            arguments: rowValues('#argumentsList'),
            wait: $('#waitForInstaller').is(':checked'),
            successExitCodes: rowValues('#exitCodesList').map(toExitCode).filter(function (code, index, codes) {
                return codes.indexOf(code) === index;
            }),
        };
    }

    function toExitCode(text) {
        const code = Number(text);
        if (!/^-?\d+$/.test(text) || !Number.isSafeInteger(code)) {
            throw new Error('Každý úspěšný návratový kód musí být celé číslo.');
        }
        return code;
    }

    /**
     * Applies the client's rules to the form: the installer type must match
     * the file, MSI and disk images always wait, and disk images show the
     * autorun.inf hint.
     */
    function refreshInstallerRules() {
        const file = selectedFile();
        const editing = editedAppId() !== null;
        const fileType = editing ? $('#appForm').data('installerType') : helpers.installerTypeFromFileName(file && file.name);
        const installerType = $('#installerType').val();
        const fileName = $('#fileName').val().trim() || (file ? file.name : '');
        const resolvedType = helpers.resolveInstallerType(installerType, fileName);
        const mustWait = helpers.installerTypeRequiresWait(resolvedType);

        if (mustWait) {
            $('#waitForInstaller').prop('checked', true);
        }
        $('#waitForInstaller').prop('disabled', mustWait);
        $('#waitRequiredHint').toggleClass('d-none', !mustWait);
        $('#diskImageHint').toggleClass('d-none', !helpers.isDiskImageType(resolvedType));

        let fileProblem = '';
        if (!editing) {
            fileProblem = !file ? 'Vyberte instalační soubor EXE, MSI, ISO nebo IMG.'
                : !fileType ? 'Nahrát lze pouze soubor .exe, .msi, .iso nebo .img.' : '';
        }
        const typeMismatch = Boolean(fileType) && resolvedType !== fileType;
        setProblem('#installerFile', fileProblem);
        setProblem('#installerType', typeMismatch && installerType !== 'auto' ? 'Zvolený typ instalátoru neodpovídá souboru .' + fileType + '.' : '');
        setProblem('#fileName', typeMismatch && installerType === 'auto'
            ? 'Při automatickém typu musí název končit příponou .' + fileType + ', jinak WinFly nepozná typ instalátoru.' : '');
    }

    function setProblem(selector, message) {
        $(selector)[0].setCustomValidity(message);
        $(selector + 'Feedback').text(message);
    }

    /** Fills in fileName from the chosen file, unless the administrator typed their own. */
    function fillFileNameFromChosenFile() {
        const file = selectedFile();
        const current = $('#fileName').val();
        if (file && (current.trim() === '' || current === autoFileName)) {
            $('#fileName').val(file.name);
            autoFileName = file.name;
        }
        hideUploadProgress();
        refreshInstallerRules();
    }

    /** Disables the fields and buttons while saving. cancelLabel: text of the enabled cancel button, or '' to disable it. */
    function setBusy(busy, cancelLabel) {
        $('#appFormFields').prop('disabled', busy);
        WinFly.ui.setButtonBusy($('#saveButton'), busy);
        $('#cancelAppButton').prop('disabled', busy && !cancelLabel).text(busy ? cancelLabel || 'Čekejte…' : 'Zrušit');
    }

    function showError(message) {
        $('#formError').toggleClass('d-none', !message).text(message);
    }

    function showUploadProgress(progress) {
        const percent = Math.max(0, Math.min(100, progress.percent || 0));
        $('#uploadProgress').removeClass('d-none');
        $('#uploadStatus').text(progress.status);
        $('#uploadPercent').text(Math.floor(percent) + ' %');
        $('#uploadDetails').text(progress.details);
        $('#uploadBar').css('width', percent.toFixed(2) + '%');
        $('#uploadProgress .progress').attr('aria-valuenow', Math.floor(percent));
    }

    function hideUploadProgress() {
        $('#uploadProgress').addClass('d-none');
    }

    // Lists of single values (arguments, exit codes). The list element's
    // data-input-type, data-placeholder and data-label describe its inputs.

    function setRows(listSelector, values) {
        $(listSelector).empty();
        values.forEach(function (value) {
            addRow(listSelector, value);
        });
        keepOneRow(listSelector);
    }

    function addRow(listSelector, value) {
        const $list = $(listSelector);
        const $input = $('<input>', {
            type: $list.data('inputType'),
            class: 'form-control form-control-sm font-monospace',
            placeholder: $list.data('placeholder'),
            'aria-label': $list.data('label'),
        }).val(value);
        const $remove = $('<button>', {type: 'button', class: 'btn btn-sm btn-outline-secondary', 'data-remove-row': '', text: 'Odebrat'});

        $list.append($('<div>', {class: 'repeater-row'}).append($input, $remove));
    }

    function removeRow(button) {
        const $row = $(button).closest('.repeater-row');
        const listSelector = '#' + $row.parent().attr('id');
        $row.remove();
        keepOneRow(listSelector);
    }

    function keepOneRow(listSelector) {
        if ($(listSelector).children().length === 0) {
            addRow(listSelector, '');
        }
    }

    function rowValues(listSelector) {
        return $(listSelector).find('input').map(function () {
            return this.value.trim();
        }).get().filter(function (value) {
            return value !== '';
        });
    }

    function bindEvents() {
        $('#installerFile').on('change', fillFileNameFromChosenFile);
        $('#installerType, #fileName').on('input change', refreshInstallerRules);
        $('#fileName').on('input', function () {
            if (this.value !== autoFileName) {
                autoFileName = null;
            }
        });
        $('#appForm').on('click', '[data-add-row]', function () {
            addRow($(this).data('addRow'), '');
        }).on('click', '[data-remove-row]', function () {
            removeRow(this);
        });
    }

    WinFly.appForm = {
        bindEvents: bindEvents,
        openForCreate: openForCreate,
        openForEdit: openForEdit,
        reset: reset,
        editedAppId: editedAppId,
        selectedFile: selectedFile,
        validate: validate,
        read: read,
        setBusy: setBusy,
        showError: showError,
        showUploadProgress: showUploadProgress,
    };
}(window.WinFly, window.jQuery));
