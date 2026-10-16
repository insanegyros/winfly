<?php defined('WINFLY_ADMIN') || exit; ?>
<div class="modal fade" id="appModal" tabindex="-1" aria-labelledby="appModalTitle" aria-hidden="true">
    <div class="modal-dialog modal-xl modal-dialog-scrollable">
        <form class="modal-content" id="appForm" novalidate>
            <div class="modal-header">
                <div>
                    <h2 class="modal-title fs-5" id="appModalTitle">Přidat aplikaci</h2>
                    <div class="small text-body-secondary" id="appModalSubtitle"></div>
                </div>
                <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Zavřít"></button>
            </div>

            <div class="modal-body">
            <fieldset id="appFormFields">
                <div id="formError" class="alert alert-danger d-none" role="alert"></div>
                <input type="hidden" id="appId" value="">

                <div class="row g-4">
                    <div class="col-lg-7">
                        <div class="mb-3">
                            <label for="appName" class="form-label">Název <span class="text-danger">*</span></label>
                            <input type="text" class="form-control" id="appName" required>
                            <div class="invalid-feedback">Zadejte název aplikace.</div>
                        </div>

                        <div class="mb-3">
                            <label for="appDescription" class="form-label">Popis</label>
                            <textarea class="form-control" id="appDescription" rows="4"></textarea>
                        </div>

                        <div class="mb-3" id="installerUploadGroup">
                            <label for="installerFile" class="form-label">
                                Instalační soubor <span class="text-danger">*</span>
                            </label>
                            <input type="file" class="form-control" id="installerFile" accept=".exe,.msi,.iso,.img" required>
                            <div class="form-text upload-note">
                                Povolené jsou soubory <code>.exe</code>, <code>.msi</code>, <code>.iso</code> a <code>.img</code>.
                                Soubor se nahrává po malých blocích, takže může být větší než limity
                                <code>post_max_size</code> a <code>upload_max_filesize</code>.
                                Výsledná URL bude <code><?= h($settings['installerBaseUrl'] . '<název-souboru>') ?></code>.
                                Typ instalátoru se určí podle přípony a do počítače uživatele se soubor stáhne pod stejným názvem.
                            </div>
                            <div class="invalid-feedback" id="installerFileFeedback"></div>

                            <div id="uploadProgress" class="mt-3 d-none" aria-live="polite">
                                <div class="d-flex justify-content-between align-items-center gap-3 small mb-1">
                                    <span id="uploadStatus"></span>
                                    <strong id="uploadPercent">0 %</strong>
                                </div>
                                <div class="progress upload-progress" role="progressbar" aria-label="Průběh nahrávání instalátoru"
                                     aria-valuemin="0" aria-valuemax="100" aria-valuenow="0">
                                    <div class="progress-bar progress-bar-striped progress-bar-animated" id="uploadBar"></div>
                                </div>
                                <div class="form-text mt-1" id="uploadDetails"></div>
                            </div>
                        </div>

                        <div class="alert alert-info small mt-3 mb-0 d-none" id="diskImageHint">
                            Obraz ISO/IMG musí mít v kořeni soubor <code>autorun.inf</code>, jehož položka
                            <code>open=</code> nebo <code>shellexecute=</code> spouští instalátor <code>.exe</code> nebo
                            <code>.msi</code> uvnitř obrazu. Argumenty zadané zde se přidají za parametry z <code>autorun.inf</code>.
                        </div>

                        <div class="mt-3">
                            <label for="sha256" class="form-label">Kontrolní součet SHA-256</label>
                            <input type="text" class="form-control font-monospace" id="sha256" maxlength="64"
                                   pattern="[A-Fa-f0-9]{64}" placeholder="Volitelný 64znakový hexadecimální kontrolní součet">
                            <div class="invalid-feedback">Zadejte přesně 64 hexadecimálních znaků, nebo pole ponechte prázdné.</div>
                        </div>

                        <div class="form-check form-switch mt-4">
                            <input class="form-check-input" type="checkbox" role="switch" id="waitForInstaller" checked>
                            <label class="form-check-label" for="waitForInstaller">Počkat na dokončení procesu instalátoru</label>
                            <div class="form-text d-none" id="waitRequiredHint">MSI a obrazy disku ISO/IMG vyžadují čekání na dokončení.</div>
                        </div>
                    </div>

                    <div class="col-lg-5">
                        <section class="repeater-card mb-4" aria-labelledby="argumentsHeading">
                            <div class="d-flex justify-content-between align-items-center gap-2 mb-3">
                                <div>
                                    <h3 class="h6 mb-1" id="argumentsHeading">Argumenty</h3>
                                    <p class="small text-body-secondary mb-0">Jeden argument příkazového řádku na každý řádek.</p>
                                </div>
                                <button type="button" class="btn btn-sm btn-outline-primary" data-add-row="#argumentsList">Přidat argument</button>
                            </div>
                            <div id="argumentsList" class="d-grid gap-2"
                                 data-input-type="text" data-placeholder="/quiet" data-label="Argument instalátoru"></div>
                        </section>

                        <section class="repeater-card" aria-labelledby="exitCodesHeading">
                            <div class="d-flex justify-content-between align-items-center gap-2 mb-3">
                                <div>
                                    <h3 class="h6 mb-1" id="exitCodesHeading">Úspěšné návratové kódy</h3>
                                    <p class="small text-body-secondary mb-0">Celočíselné návratové kódy procesu považované za úspěch.</p>
                                </div>
                                <button type="button" class="btn btn-sm btn-outline-primary" data-add-row="#exitCodesList">Přidat kód</button>
                            </div>
                            <div id="exitCodesList" class="d-grid gap-2"
                                 data-input-type="number" data-placeholder="0" data-label="Úspěšný návratový kód"></div>
                        </section>
                    </div>
                </div>
            </fieldset>
            </div>

            <div class="modal-footer">
                <button type="button" class="btn btn-outline-secondary" id="cancelAppButton">Zrušit</button>
                <button type="submit" class="btn btn-primary" id="saveButton">
                    <span class="button-label">Přidat aplikaci</span>
                    <span class="spinner-border spinner-border-sm ms-2 d-none" role="status" aria-hidden="true"></span>
                </button>
            </div>
        </form>
    </div>
</div>
