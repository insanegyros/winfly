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
| Download | `internal/download/downloader.go` | `Manager.Fetch` streams to `<cache>\<fileName>.*.part`, hashes while writing, compares SHA-256, renames into place, reuses a cached file whose hash matches. Cache dir ACL is locked to SYSTEM and Administrators (`cache_security.go`). Size limit `network.maxDownloadBytes` (default 8 GiB). |
| SHA-256 re-check and lock | `internal/install/filelock.go` | `lockAndVerifyInstaller` opens the file with `FILE_SHARE_READ` only, re-hashes it, and keeps the handle open while the installer runs. |
| Execution | `internal/install/installer.go` | `Run` → `installerKind` (type from `installerType` or file extension) → `commandFor` (EXE runs directly, MSI via `%SystemRoot%\System32\msiexec.exe /i`) → `exec.Cmd`, exit code checked against `successExitCodes`, 1641/3010 mean restart. |
| Logging | `install.Logger`, `download.Logger` | Both are `func(format string, args ...any)`; the UI passes `app.logf`, which writes to the log window. |
| GUI integration | `internal/ui/installation.go` | `installApp` calls `downloader.Fetch` then `install.Run` on a background goroutine; the UI only sees the joined error and the restart flag. |

Before step 2a, two places decided "what kind of file is this":
`installerKind` in `install` and the `installerType` switch in
`config.validateApps`. Step 2a replaced both with one shared function,
`config.ResolveInstallerType` (`internal/config/installer_type.go`), which
also knows `iso` and `img`.

## Design

### Config (remote JSON)

- `installerType` gains `iso` and `img`. `auto` also detects `.iso` and `.img`
  from `fileName`.
- No new config field. The program to run comes from `autorun.inf` in the
  image root (see "Autorun" below); this replaced the earlier `setupPath`
  idea, so the app list format and the admin panel need no new field.
- `arguments` from the app list are appended after the arguments from
  `autorun.inf` (decided in step 4).
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
  "arguments": [],
  "wait": true,
  "successExitCodes": [0, 1641, 3010]
}
```

### Mounting

Package `internal/diskimage` (step 3, independent of the GUI and of
`install`) mounts the image through `virtdisk.dll`: `OpenVirtualDisk` +
`AttachVirtualDisk` with `VIRTUAL_STORAGE_TYPE_DEVICE_ISO`, read-only, without
permanent lifetime. No PowerShell or other program is started, so there is no
command string to inject into. `virtdisk.dll` is loaded from System32 only.

- `Mounter.Mount(ctx, path)` checks the file, mounts it and waits (up to 30 s,
  cancellable through `ctx`) for Windows to create the volume. It returns an
  `Image` with `MountPath` (`E:\`, or `\\?\Volume{…}\` when Windows assigns
  no drive letter) and `Owned`.
- `Image.Unmount()` detaches only an image WinFly mounted itself (`Owned`).
  If the user or another app already had the image mounted, Mount reuses that
  mount with `Owned=false` and Unmount only closes WinFly's handle. Because
  the attach is not permanent, Windows also detaches WinFly's mount when the
  handle closes, even if WinFly crashes.
- The volume is found by matching the device number of the mounted CD-ROM
  device (`GetVirtualDiskPhysicalPath`) against every volume
  (`FindFirstVolumeW`, `IOCTL_STORAGE_GET_DEVICE_NUMBER`), then
  `GetVolumePathNamesForVolumeNameW` for the drive letter.
- `SeManageVolumePrivilege` is enabled before attaching.
- Errors for callers to tell apart with `errors.Is`: `ErrImageNotFound`,
  `ErrAccessDenied`, `ErrUnsupportedFormat`, `ErrNoVolume`, and the context
  errors on cancellation.
- The Windows calls sit behind a small `system` interface; tests use a fake,
  so the mount, ownership, cancellation and error logic is tested on any OS.

Formats (`format.go`): the file content decides, not the extension. Accepted
are ISO 9660 (`CD001`) and UDF (`NSR02`/`NSR03`) images, which is what Windows
can mount; this covers ISO-compatible `.img` files. Rejected with a specific
Czech error: hard disk / USB / SD card images (boot signature `55AA`), raw
2352-byte-sector CD images (CloneCD BIN/IMG), files that are too small and
anything else without ISO 9660 or UDF.

### Autorun

`diskimage.ReadAutorun(mountPath)` (`autorun.go`, `cmdline.go`) validates the
installer of a mounted image and returns an `AutorunInstaller` (full `Path`,
`RelativePath`, `Arguments`, `Type` exe/msi, `Entry` open/shellexecute). It
never executes anything.

- `autorun.inf` is looked up in the image root case-insensitively. Without it
  the image is rejected (`ErrNoAutorun`); WinFly never guesses `setup.exe`.
- The `[AutoRun]` section is parsed (UTF-8, UTF-8 BOM or UTF-16 LE BOM,
  `;` comments, case-insensitive keys, first occurrence wins). `open=` and
  `shellexecute=` are both supported; if both are present they must match.
  Icon- or label-only files fail with `ErrNoInstallerEntry`; malformed lines,
  unterminated quotes or sections, empty entries, other programs than
  `.exe`/`.msi` and files over 64 KiB fail with `ErrInvalidAutorun`.
- The command line is split by Windows rules (quoted program names,
  quoted arguments with spaces, backslash-quote escaping).
- The path must stay inside the image: drive letters, UNC paths, `%VAR%`,
  `..`, data streams, wildcards and reserved names fail with
  `ErrUnsafeInstallerPath` (each segment is checked with the existing
  `config.IsSafeInstallerFileName`). A leading `\` means the image root.
- Each path segment is resolved case-insensitively and the target must be an
  existing regular file (`ErrInstallerNotFound`).

### Execution

`install.Run` stays the single entry point the UI calls. For `iso`/`img`:

1. Lock and verify the image with the existing `lockAndVerifyInstaller`.
2. Mount it read-only with `diskimage.Mounter`, `defer` the unmount.
3. Validate the installer with `diskimage.ReadAutorun` on the mount path.
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
| 2a | Config: `iso`/`img` installer types and constants, case-insensitive `.iso`/`.img` detection for `auto`, `wait: true` required for images, descriptive errors for unsupported types, one shared `ResolveInstallerType` used by `config` and `install`; `install.Run` stops images with a clear "not supported yet" error before touching the file; tests incl. unchanged EXE/MSI rules | `internal/config/installer_type.go` (new), `installer_type_test.go` (new), `config.go`, `internal/install/installer.go`, `installer_test.go` | ✅ |
| 2b | ~~Config: `setupPath` field~~ Dropped: replaced by autorun validation (step 3b) | — | ✅ |
| 3 | Reusable mounting component: format check, mount/unmount via `virtdisk.dll`, volume and drive letter lookup, ownership, cancellation, descriptive errors; tests with a fake OS layer. Done and tested on Linux and under Wine; real mounting still needs a test on real Windows 10/11 (step 9) | `internal/diskimage/` (new) | ✅ |
| 3b | Autorun-based installer validation: find `autorun.inf`, parse `[AutoRun]` `open`/`shellexecute`, split quoted command lines, reject unsafe paths, icon-only and malformed files, check the target exists; table-driven tests | `internal/diskimage/autorun.go`, `cmdline.go` (new), tests | ✅ |
| 4 | Run the setup program from the mounted image; split `Run` into small helpers | `internal/install/installer.go`, `internal/install/installer_test.go` | ⬜ |
| 5a | Raise the default `maxDownloadBytes` from 2 GiB to 8 GiB for big ISOs | `internal/config/config.go`, `config.json`, `internal/download/downloader.go`, `README.md` | ✅ |
| 5b | Download: accept image MIME types in the `Accept` header | `internal/download/downloader.go` | ⬜ |
| 6 | GUI: no change planned; progress goes to the log window | `internal/ui/app_details.go` | ⬜ |
| 7 | Docs: README table for `installerType`, the autorun.inf requirement; ISO example in `apps_example.json` | `README.md`, `apps_example.json` | ⬜ |
| 8 | Admin panel (postponed by Jandy, do later): allow `.iso`/`.img` uploads, `iso`/`img` types, `.iso`/`.img` file names | `admin_panel_web/ProvisionerJsonCRUD.php`, `api.php`, `ProvisionerChunkUpload.php`, `index.php`, `assets/admin.js` | ⬜ |
| 9 | Build with `build.ps1`, test on real Windows with an ISO and an IMG, both EXE and MSI inside | — | ⬜ |

## Open questions

- Step 8 is postponed: until the panel accepts `.iso`/`.img` uploads and
  file names, ISO apps can only be added by editing `apps.json` by hand.
- `network.timeoutSeconds` (default 900 s) limits the whole download, so an
  8 GiB image needs roughly 9.5 MB/s to finish in time.
- `lockAndVerifyInstaller` opens the image with `FILE_SHARE_READ` only;
  `AttachVirtualDisk` must still be able to open it read-only. Verify in step 4 on real Windows.
- Until step 4 lands, an ISO/IMG app is downloaded and SHA-256 checked
  as usual, then `install.Run` stops it with "instalace z obrazu disku ISO
  zatím není podporována". `auto` with an unknown extension (e.g. the panel's
  `app.bin`) still loads, exactly as before, and fails only when installed.
