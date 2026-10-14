<?php defined('WINFLY_ADMIN') || exit; ?>
<div class="modal fade" id="deleteModal" tabindex="-1" aria-labelledby="deleteModalTitle" aria-hidden="true">
    <div class="modal-dialog modal-dialog-centered">
        <div class="modal-content">
            <div class="modal-header">
                <h2 class="modal-title fs-5" id="deleteModalTitle">Smazat aplikaci</h2>
                <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Zavřít"></button>
            </div>
            <div class="modal-body">
                <p class="mb-2">Opravdu smazat aplikaci <strong id="deleteAppName"></strong>?</p>
                <p class="text-body-secondary mb-2">
                    Záznam bude trvale odstraněn z <code>apps.json</code> a současně se smaže jeho nahraný
                    instalační soubor (EXE, MSI, ISO nebo IMG) z nadřazené složky.
                </p>
                <p class="small text-warning-emphasis mb-0">Pouhé zakázání aplikace instalační soubor nemaže.</p>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-outline-secondary" data-bs-dismiss="modal">Zrušit</button>
                <button type="button" class="btn btn-danger" id="confirmDeleteButton">
                    <span class="button-label">Smazat</span>
                    <span class="spinner-border spinner-border-sm ms-2 d-none" role="status" aria-hidden="true"></span>
                </button>
            </div>
        </div>
    </div>
</div>
