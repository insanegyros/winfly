// Calls to api.php. Every call returns a Promise with an extra abort() method.
// A failed call rejects with an Error that also has status, code, hasJsonBody,
// aborted and timedOut; its message is the server's message or empty.
(function (WinFly, $) {
    'use strict';

    const config = window.WinFlyConfig || {};

    /**
     * options: method ('POST'), query (object), json (request body as JSON),
     * body (Blob sent as raw bytes), timeout (ms), onUploadProgress (listener).
     */
    function request(action, options) {
        const settings = options || {};
        const isBinary = settings.body instanceof Blob;
        let jqXhr;

        const promise = new Promise(function (resolve, reject) {
            jqXhr = $.ajax({
                url: config.apiUrl + '?' + $.param($.extend({action: action}, settings.query)),
                method: settings.method || 'POST',
                dataType: 'json',
                cache: false,
                timeout: settings.timeout || 0,
                headers: {'X-CSRF-Token': config.csrfToken},
                contentType: isBinary ? 'application/octet-stream' : 'application/json; charset=utf-8',
                data: isBinary ? settings.body : settings.json === undefined ? undefined : JSON.stringify(settings.json),
                processData: false,
                xhr: function () {
                    const xhr = $.ajaxSettings.xhr();
                    if (settings.onUploadProgress && xhr.upload) {
                        xhr.upload.addEventListener('progress', settings.onUploadProgress);
                    }
                    return xhr;
                },
            });

            jqXhr.done(resolve).fail(function (xhr, textStatus) {
                if (xhr.status === 401) {
                    window.location.href = config.loginUrl;
                }
                reject(toRequestError(xhr, textStatus));
            });
        });

        promise.abort = function () {
            jqXhr.abort();
        };
        return promise;
    }

    function toRequestError(xhr, textStatus) {
        const body = xhr.responseJSON || parseJsonOrNull(xhr.responseText);
        const error = new Error(body && typeof body.message === 'string' ? body.message : '');

        error.status = Number(xhr.status) || 0;
        error.code = body && typeof body.code === 'string' ? body.code : '';
        error.hasJsonBody = Boolean(body);
        error.aborted = textStatus === 'abort';
        error.timedOut = textStatus === 'timeout';
        return error;
    }

    function parseJsonOrNull(text) {
        try {
            return JSON.parse(text);
        } catch (error) {
            return null;
        }
    }

    /** The message to show for a failed call. */
    function errorMessage(error, fallback) {
        return error && error.message ? error.message : fallback;
    }

    WinFly.api = {
        request: request,
        get: function (action, query) {
            return request(action, {method: 'GET', query: query});
        },
        post: function (action, json) {
            return request(action, {json: json});
        },
        errorMessage: errorMessage,
    };
}(window.WinFly, window.jQuery));
