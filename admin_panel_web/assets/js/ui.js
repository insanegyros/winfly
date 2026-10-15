// Messages and busy states shared by the page and its dialogs.
(function (WinFly, $, bootstrap) {
    'use strict';

    const TOAST_CLASSES = {success: 'text-bg-success', warning: 'text-bg-warning', danger: 'text-bg-danger'};

    /** Shows a message in the top right corner for a few seconds. type: success, warning or danger. */
    function showToast(message, type) {
        const closeButtonClass = type === 'warning' ? 'btn-close' : 'btn-close btn-close-white';
        const $toast = $('<div>', {class: 'toast align-items-center border-0 ' + TOAST_CLASSES[type], role: 'status'}).append(
            $('<div>', {class: 'd-flex'}).append(
                $('<div>', {class: 'toast-body'}).text(message),
                $('<button>', {type: 'button', class: closeButtonClass + ' me-2 m-auto', 'data-bs-dismiss': 'toast', 'aria-label': 'Zavřít'})
            )
        );

        $('#toastContainer').append($toast);
        const toast = new bootstrap.Toast($toast[0], {delay: 5500});
        $toast.on('hidden.bs.toast', function () {
            toast.dispose();
            $toast.remove();
        });
        toast.show();
    }

    /** Shows or, with an empty message, hides the alert above the app list. */
    function setPageAlert(message, type) {
        $('#pageAlert')
            .removeClass('alert-success alert-warning alert-danger')
            .addClass(message ? 'alert-' + type : '')
            .toggleClass('d-none', !message)
            .text(message || '');
    }

    /** Disables a button and shows its spinner. */
    function setButtonBusy($button, busy) {
        $button.prop('disabled', busy).find('.spinner-border').toggleClass('d-none', !busy);
    }

    /** Replaces a small table button's label with a spinner while busy. */
    function setInlineButtonBusy($button, busy) {
        if (busy) {
            $button.data('label', $button.html()).prop('disabled', true)
                .html('<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span>');
            return;
        }
        $button.prop('disabled', false).html($button.data('label'));
    }

    WinFly.ui = {
        showToast: showToast,
        setPageAlert: setPageAlert,
        setButtonBusy: setButtonBusy,
        setInlineButtonBusy: setInlineButtonBusy,
    };
}(window.WinFly, window.jQuery, window.bootstrap));
