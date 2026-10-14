# WinFly: ISO/IMG installer support

Goal: let an app in the remote list point to an `.iso` or `.img` disk image.
WinFly downloads the image, verifies it, mounts it read-only, runs the setup
program inside it (EXE or MSI), waits for it, and unmounts the image.

This extends the existing pipeline. No new application, no redesign, and the
GUI in `internal/ui` stays as it is. DxDiag information, Power Options and
Domain/Workgroup functionality are not touched.

Code follows the `golang-code-style` skill (samber/cc-skills-golang): early
returns, no needless `else`, `switch` over if-else chains, named fields in
composite literals, short focused functions, at most 4 parameters, user-facing
texts in Czech.

## Current pipeline

| Stage | Where | What it does |
|---|---|---|
| Embedded config | `internal/config/config.go`, `config.json` | Product name, cache dir, remote URL, network limits. |
| Remote app list | `internal/config/remote.go`, `config.go` | GET the JSON, strict decode (`DisallowUnknownFields`), defaults in `applyAppDefaults`, rules in `validateApps`. `installerType` must be `auto`, `exe` or `msi`. |
| Download | `internal/download/downloader.go` | `Manager.Fetch` streams to `<cache>\<fileName>.*.part`, hashes while writing, compares SHA-256, renames into place, reuses a cached file whose hash matches. Cache dir ACL is locked to SYSTEM and Administrators (`cache_security.go`). Size limit `network.maxDownloadBytes` (default 2 GiB). |
| SHA-256 re-check and lock | `internal/install/filelock.go` | `lockAndVerifyInstaller` opens the file with `FILE_SHARE_READ` only, re-hashes it, and keeps the handle open while the installer runs. |
| Execution | `internal/install/installer.go` | `Run` → `installerKind` (type from `installerType` or file extension) → `commandFor` (EXE runs directly, MSI via `%SystemRoot%\System32\msiexec.exe /i`) → `exec.Cmd`, exit code checked against `successExitCodes`, 1641/3010 mean restart. |
| Logging | `install.Logger`, `download.Logger` | Both are `func(format string, args ...any)`; the UI passes `app.logf`, which writes to the log window. |
| GUI integration | `internal/ui/installation.go` | `installApp` calls `downloader.Fetch` then `install.Run` on a background goroutine; the UI only sees the joined error and the restart flag. |

The two package-level points that decide "what kind of file is this" are
`installerKind` in `install` and the `installerType` switch in
`config.validateApps`. ISO/IMG support plugs in there.

## Design

### Config (remote JSON)

- `installerType` gains `iso` and `img`. `auto` also detects `.iso` and `.img`
  from `fileName`.
- New optional field `setupPath`: the path of the setup program inside the
  image, relative to its root, e.g. `setup.exe` or `x64\setup.msi`. Required
  for `iso`/`img`, rejected for `exe`/`msi`.
- The program inside the image is EXE or MSI, decided by the extension of
  `setupPath`. `arguments` go to that program, exactly as today.
- `sha256` covers the image file itself. Nothing changes in the download step.
- Images require `wait: true` (the image must stay mounted until setup ends),
  the same rule MSI has today.

Example:

```json
{
  "id": 20,
  "name": "Office 24 (CZ)",
  "enabled": true,
  "url": "https://example.com/winfly/office24.iso",
  "sha256": "…",
  "fileName": "office24.iso",
  "installerType": "auto",
  "setupPath": "setup.exe",
  "arguments": ["/configure", "config.xml"],
  "wait": true,
  "successExitCodes": [0, 1641, 3010]
}
```

### Mounting

A new file `internal/install/diskimage.go` mounts the image through
`virtdisk.dll` (`OpenVirtualDisk` + `AttachVirtualDisk` with
`VIRTUAL_STORAGE_TYPE_DEVICE_ISO`, read-only, without permanent lifetime), the
same lazy-DLL style `filelock.go` already uses for `CreateFileW`. Because the
attach is not permanent, Windows detaches the image when the handle closes,
even if WinFly crashes. The drive letter is found from
`GetVirtualDiskPhysicalPath` and the volume mount points.

Fallback if the API proves awkward: `powershell Mount-DiskImage -PassThru |
Get-Volume`, the same `exec.Command` style `cache_security.go` uses for
`icacls.exe`. Decide in step 3 after testing on real Windows.

Note: `.img` works only when it holds an ISO 9660/UDF file system (the common
case, which Windows Explorer also mounts). Raw partitioned disk images are out
of scope and will fail with a clear Czech error.

### Execution

`install.Run` stays the single entry point the UI calls. For `iso`/`img`:

1. Lock and verify the image with the existing `lockAndVerifyInstaller`.
2. Mount it read-only (`diskimage.go`), `defer` the unmount.
3. Resolve `setupPath` inside the mounted root, refusing paths that escape it.
4. Reuse `commandFor` / the existing run-and-check-exit-code code for the inner
   EXE or MSI, with `cmd.Dir` set to the setup program's folder.
5. Log mount, setup start, result and unmount through the existing `Logger`.

To keep `Run` short, the current body is split into small helpers
(`runCommand`, `checkExitCode`) that both the plain and the image path call.

### GUI

No layout changes. The existing "Přehled aplikace" panel may show the setup
path for images in the existing "Parametry instalátoru" text, only if it is
needed for clarity. Progress already reaches the log window through `logf`.

## Steps and progress

Status: ✅ done, 🔄 in progress, ⬜ not started.

| # | Step | Files | Status |
|---|---|---|---|
| 1 | Inspect the pipeline and write this plan | `PLAN.md` | ✅ |
| 2 | Config: `iso`/`img` types, `setupPath` field, validation (safe relative path, EXE/MSI extension, `wait` required), tests | `internal/config/config.go`, `internal/config/config_test.go` | ⬜ |
| 3 | Mount and unmount a disk image via `virtdisk.dll`, find its drive letter; test on real Windows 10/11 | `internal/install/diskimage.go` (new) | ⬜ |
| 4 | Detect image kinds and run the setup program from the mounted image; split `Run` into small helpers | `internal/install/installer.go`, `internal/install/installer_test.go` | ⬜ |
| 5 | Download: accept image MIME types in the `Accept` header; consider a larger default `maxDownloadBytes` for big ISOs | `internal/download/downloader.go`, `internal/config/config.go` | ⬜ |
| 6 | GUI: optional setup path display in the existing details panel, no other change | `internal/ui/app_details.go` | ⬜ |
| 7 | Docs: README table for `installerType`, `setupPath`; ISO example in `apps_example.json` | `README.md`, `apps_example.json` | ⬜ |
| 8 | Admin panel (needs Jandy's go-ahead): allow `.iso`/`.img` uploads, `iso`/`img` types and the `setupPath` field | `admin_panel_web/ProvisionerJsonCRUD.php`, `api.php`, `ProvisionerChunkUpload.php`, `index.php`, `assets/admin.js` | ⬜ |
| 9 | Build with `build.ps1`, test on real Windows with an ISO and an IMG, both EXE and MSI inside | — | ⬜ |

## Open questions

- Step 8: the remote JSON is decoded strictly, so the panel must know
  `setupPath` before an ISO app can be saved through it. Until then such apps
  can only be added by editing `apps.json` by hand.
- `maxDownloadBytes` default is 2 GiB; many Office or OS ISOs are larger.
- `lockAndVerifyInstaller` opens the image with `FILE_SHARE_READ` only;
  `AttachVirtualDisk` must still be able to open it read-only. Verify in step 3.
