// Draws the app list table. The buttons carry data-action and data-id; main.js
// handles their clicks.
(function (WinFly, $) {
    'use strict';

    const helpers = WinFly.helpers;
    const escapeHtml = helpers.escapeHtml;

    /** Draws the apps that match the search text and updates the counter. */
    function renderApps(apps, searchText) {
        const query = searchText.trim().toLocaleLowerCase('cs-CZ');
        const shownApps = apps.filter(function (app) {
            return query === '' || searchableText(app).includes(query);
        });

        $('#appCount').text(query === '' ? apps.length : shownApps.length + ' / ' + apps.length);

        if (shownApps.length > 0) {
            $('#appsTableBody').html(shownApps.map(appRowHtml).join(''));
        } else if (query !== '') {
            renderMessageRow('Žádná aplikace neodpovídá hledání', 'Změňte hledaný výraz a zobrazte další záznamy.');
        } else {
            renderMessageRow('Nejsou nastavené žádné aplikace', 'Přidejte první instalační záznam do apps.json.',
                '<button type="button" class="btn btn-primary mt-3" data-action="create">Přidat aplikaci</button>');
        }
    }

    function renderLoading() {
        $('#appCount').text('…');
        $('#appsTableBody').html(
            '<tr><td colspan="3" class="text-center py-5 text-body-secondary">'
            + '<div class="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></div>'
            + 'Načítám aplikace…</td></tr>'
        );
    }

    function renderLoadError(message) {
        $('#appCount').text('0');
        renderMessageRow('Aplikace se nepodařilo načíst', message,
            '<button type="button" class="btn btn-outline-primary mt-3" data-action="reload">Zkusit znovu</button>', 'text-danger');
    }

    function renderMessageRow(heading, text, buttonHtml, headingClass) {
        $('#appsTableBody').html(
            '<tr><td colspan="3" class="text-center py-5"><div class="empty-state">'
            + '<h3 class="h6 mb-2 ' + (headingClass || '') + '">' + escapeHtml(heading) + '</h3>'
            + '<p class="text-body-secondary mb-0">' + escapeHtml(text) + '</p>'
            + (buttonHtml || '')
            + '</div></td></tr>'
        );
    }

    function searchableText(app) {
        return [app.id, app.name, app.description, app.url, app.fileName, app.installerType]
            .join(' ')
            .toLocaleLowerCase('cs-CZ');
    }

    function appRowHtml(app) {
        const id = escapeHtml(app.id);
        const enabled = app.enabled === true;
        const installerType = String(app.installerType || 'auto');

        return '<tr>'
            + '<td><div class="d-flex align-items-start gap-2">'
            + '<span class="status-dot ' + (enabled ? 'status-dot-enabled' : 'status-dot-disabled') + ' mt-2" aria-hidden="true"></span>'
            + '<div class="min-w-0">'
            + '<div class="d-flex flex-wrap align-items-center gap-2 mb-1">'
            + '<strong class="app-name">' + escapeHtml(app.name || '') + '</strong>'
            + (enabled
                ? '<span class="badge text-bg-success">Povoleno</span>'
                : '<span class="badge text-bg-secondary">Zakázáno</span>')
            + '</div>'
            + '<div class="small text-body-secondary">ID #' + id
            + ' &middot; ' + (installerType === 'auto' ? 'AUTOMATICKY' : escapeHtml(installerType.toUpperCase()))
            + (app.fileName ? ' &middot; <span class="app-file font-monospace">' + escapeHtml(app.fileName) + '</span>' : '')
            + '</div></div></div></td>'
            + '<td><div class="app-description mb-1">'
            + (app.description ? helpers.formatMultilineText(app.description) : '<span class="text-body-secondary fst-italic">Bez popisu</span>')
            + '</div>' + urlHtml(String(app.url || '')) + '</td>'
            + '<td class="text-end"><div class="action-buttons">'
            + '<button type="button" class="btn btn-sm btn-outline-primary" data-action="edit" data-id="' + id + '">Upravit</button>'
            + (enabled
                ? '<button type="button" class="btn btn-sm btn-outline-warning" data-action="disable" data-id="' + id + '">Zakázat</button>'
                : '<button type="button" class="btn btn-sm btn-outline-success" data-action="enable" data-id="' + id + '">Povolit</button>')
            + '<button type="button" class="btn btn-sm btn-outline-danger" data-action="delete" data-id="' + id + '">Smazat</button>'
            + '</div></td>'
            + '</tr>';
    }

    /** A link for a valid http(s) URL; anything else is shown in red as text. */
    function urlHtml(url) {
        const safeUrl = helpers.safeHttpUrl(url);
        if (safeUrl) {
            return '<a class="app-url small font-monospace link-secondary" href="' + escapeHtml(safeUrl)
                + '" target="_blank" rel="noopener noreferrer">' + escapeHtml(url) + '</a>';
        }
        return url ? '<span class="app-url small font-monospace text-danger">' + escapeHtml(url) + '</span>' : '';
    }

    WinFly.appTable = {
        renderApps: renderApps,
        renderLoading: renderLoading,
        renderLoadError: renderLoadError,
    };
}(window.WinFly, window.jQuery));
