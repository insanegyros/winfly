// Small helpers shared by the other scripts: installer type rules, formatting
// and escaping. The installer type rules mirror src/installer_types.php.
window.WinFly = window.WinFly || {};

(function (WinFly) {
    'use strict';

    const INSTALLER_EXTENSIONS = ['exe', 'msi', 'iso', 'img'];
    const TYPES_REQUIRING_WAIT = ['msi', 'iso', 'img'];
    const DISK_IMAGE_TYPES = ['iso', 'img'];

    /** Returns exe, msi, iso or img by the file extension (any letter case), or null. */
    function installerTypeFromFileName(fileName) {
        const match = /\.([^.]+)$/.exec(String(fileName || ''));
        const extension = match ? match[1].toLowerCase() : '';
        return INSTALLER_EXTENSIONS.includes(extension) ? extension : null;
    }

    /** Returns the installer type the WinFly client will use, or null. */
    function resolveInstallerType(installerType, fileName) {
        return installerType === 'auto' ? installerTypeFromFileName(fileName) : installerType;
    }

    function installerTypeRequiresWait(resolvedType) {
        return TYPES_REQUIRING_WAIT.includes(resolvedType);
    }

    function isDiskImageType(resolvedType) {
        return DISK_IMAGE_TYPES.includes(resolvedType);
    }

    /** Returns the installer type of the file a URL points to, or null. */
    function installerTypeFromUrl(url) {
        try {
            return installerTypeFromFileName(decodeURIComponent(new URL(url).pathname));
        } catch (error) {
            return null;
        }
    }

    /** Returns the URL when it is a valid http(s) URL, otherwise an empty string. */
    function safeHttpUrl(value) {
        try {
            const url = new URL(String(value));
            return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : '';
        } catch (error) {
            return '';
        }
    }

    function escapeHtml(value) {
        return String(value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }

    function formatMultilineText(value) {
        return escapeHtml(value).replace(/\r?\n/g, '<br>');
    }

    /** Formats a byte count like "1,5 MiB". */
    function formatBytes(value) {
        const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
        let size = Math.max(0, Number(value) || 0);
        let unitIndex = 0;

        while (size >= 1024 && unitIndex < units.length - 1) {
            size /= 1024;
            unitIndex += 1;
        }

        const decimals = unitIndex === 0 || size >= 100 ? 0 : size >= 10 ? 1 : 2;
        return size.toLocaleString('cs-CZ', {maximumFractionDigits: decimals}) + ' ' + units[unitIndex];
    }

    function delay(milliseconds) {
        return new Promise(function (resolve) {
            window.setTimeout(resolve, milliseconds);
        });
    }

    WinFly.helpers = {
        installerTypeFromFileName: installerTypeFromFileName,
        resolveInstallerType: resolveInstallerType,
        installerTypeRequiresWait: installerTypeRequiresWait,
        isDiskImageType: isDiskImageType,
        installerTypeFromUrl: installerTypeFromUrl,
        safeHttpUrl: safeHttpUrl,
        escapeHtml: escapeHtml,
        formatMultilineText: formatMultilineText,
        formatBytes: formatBytes,
        delay: delay,
    };
}(window.WinFly));
