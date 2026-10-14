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

`install.Run` stays the single entry point the UI calls; there is no
separate installer engine. For `iso`/`img` (`internal/install/image.go`):

1. Check `wait: true`, then lock and verify the image with the existing
   `lockAndVerifyInstaller`. The lock is held until everything below ends.
2. Mount it read-only with `diskimage.Mounter`. An image another app already
   mounted is reused and left mounted.
3. Validate the installer with `diskimage.ReadAutorun` on the mount root.
4. `imageLaunch` builds the command with the existing `commandFor`: an EXE
   runs directly, an MSI through `%SystemRoot%\System32\msiexec.exe /i`.
   Arguments are the `autorun.inf` arguments followed by the app's
   `arguments`, each kept as a separate argument (Go quotes them for
   Windows). No silent switches are ever added; unattended installs work
   only when `autorun.inf` or the app list provides the switches.
5. The working directory is the image root, as with Windows AutoPlay, since
   relative paths in `autorun.inf` are relative to the root.
6. The shared `runCommand` (split out of `Run`, used by plain EXE/MSI too)
   waits for the installer and checks `successExitCodes`; 1641/3010 still
   mean restart.
7. The image is unmounted in a `defer`, so only after the installer process
   has exited, also on failure or cancellation. An unmount failure is logged
   as a warning and does not fail an otherwise successful install.
8. Mount, mount path, the autorun entry, start, result and unmount are logged
   through the existing `Logger`, so they appear in the log window.

Limit: WinFly waits for the process it started. An installer that starts a
child process and exits at once would see the image unmounted under it;
such installers need their own "wait" switch in `arguments`.

### Workflow integration

The full sequence for an ISO/IMG app is: download (`download.Fetch`) →
SHA-256 check (download, then again under the lock in `install.Run`) →
mount → validate `autorun.inf` → run the installer → wait → unmount.

- **Selected and batch installs:** `ui/installation.go` already calls
  `downloader.Fetch` then `install.Run` per selected app, one after another,
  and logs and skips a failed app. Images use exactly this path; the GUI
  holds no image logic.
- **Nothing runs early:** the installer is started only after mounting and
  autorun validation succeeded. A corrupted image fails the SHA-256 check;
  an unsupported, unmountable or autorun-less image stops only that app
  with an `install.ImageError` (`Stage`: lock, mount or autorun; the cause
  still matches `errors.Is`, e.g. `diskimage.ErrNoAutorun`).
- **Unmounting:** only images WinFly mounted itself are unmounted, after the
  installer exits, on success, failure and cancellation (`defer`). If the
  explicit detach fails, closing the handle still detaches a WinFly-owned
  image because the attach is not permanent.
- **`wait=false`:** never allowed for images (the image would be unmounted
  under a running installer). The app list rejects it when loading, the same
  rule MSI already has, and `install.Run` refuses it again before mounting.
- **Same image twice / concurrent jobs:** `lockImage` lets only one install
  use a given image file at a time (case-insensitive path), so one job can
  never unmount an image another job is installing from, and an image is
  never mounted twice by WinFly. Waiting can be cancelled through `ctx`.
  The UI itself also runs only one batch at a time (`installing` flag).
- **Remote files and catalog:** WinFly only downloads; it never deletes or
  changes remote files or app list entries.
- **Download cleanup:** unchanged policy. A failed download's `.part` file is
  removed; a verified image stays in the cache and is reused next time when
  its SHA-256 matches, exactly like EXE/MSI files.

### GUI

Minimal, targeted changes; `ui.go` itself is unchanged, and the DxDiag,
Power Options and Domain/Workgroup code is not touched. The state lives
outside the GUI in `install.ImageStatuses` (`internal/install/image_status.go`).

- **List and details:** ISO/IMG apps appear in the same list as EXE/MSI.
  The "Přehled aplikace" panel has one new row, "Typ instalátoru", e.g.
  `EXE`, `MSI` or `Obraz disku ISO — neověřeno, ověří se před instalací`.
- **States:** every image starts as *neověřeno* (unverified). After an
  install attempt it becomes *ověřeno, lze instalovat* when mounting and
  autorun validation succeeded (even if the installer then failed), or
  *neplatný, instalace je zakázána* when the format is unsupported or
  `autorun.inf` is missing or invalid. Temporary failures (network, missing
  permission, no volume, cancellation) leave the state unchanged.
- **Blocking:** an invalid image is skipped before it is downloaded again,
  with its message in the result dialog. The state is kept per image
  (app ID, URL, SHA-256, file name) for the running program; a changed image
  in the app list starts unverified again, and restarting WinFly revalidates.
- **Messages:** the result dialog shows the fixed Czech texts
  (`install.MessageNoAutorun`, `MessageInvalidAutorun`,
  `MessageUnsupportedFormat`, picked by `install.UserMessage`); the log keeps
  the full technical error.
- **Not blocking the GUI:** download, mount, validation and installation keep
  running on the existing background goroutine; the panel refreshes when the
  batch finishes.

Screenshots under Wine: `iso-gui/` in the project files.

## Steps and progress

Status: ✅ done, 🔄 in progress, ⬜ not started.

| # | Step | Files | Status |
|---|---|---|---|
| 1 | Inspect the pipeline and write this plan | `PLAN.md` | ✅ |
| 2a | Config: `iso`/`img` installer types and constants, case-insensitive `.iso`/`.img` detection for `auto`, `wait: true` required for images, descriptive errors for unsupported types, one shared `ResolveInstallerType` used by `config` and `install`; `install.Run` stops images with a clear "not supported yet" error before touching the file; tests incl. unchanged EXE/MSI rules | `internal/config/installer_type.go` (new), `installer_type_test.go` (new), `config.go`, `internal/install/installer.go`, `installer_test.go` | ✅ |
| 2b | ~~Config: `setupPath` field~~ Dropped: replaced by autorun validation (step 3b) | — | ✅ |
| 3 | Reusable mounting component: format check, mount/unmount via `virtdisk.dll`, volume and drive letter lookup, ownership, cancellation, descriptive errors; tests with a fake OS layer. Done and tested on Linux and under Wine; real mounting still needs a test on real Windows 10/11 (step 9) | `internal/diskimage/` (new) | ✅ |
| 3b | Autorun-based installer validation: find `autorun.inf`, parse `[AutoRun]` `open`/`shellexecute`, split quoted command lines, reject unsafe paths, icon-only and malformed files, check the target exists; table-driven tests | `internal/diskimage/autorun.go`, `cmdline.go` (new), tests | ✅ |
| 4 | Run the autorun installer from the mounted image through the existing EXE/MSI path; `runCommand` shared with plain installers; image kept mounted until the installer exits; tests incl. a real process launch (test binary as fake installer) | `internal/install/image.go` (new), `installer.go`, `image_test.go` (new), `installer_test.go` | ✅ |
| 4b | Workflow integration: per-image lock against concurrent/duplicate use, `ImageError` with the failing stage, checks of batch, cancellation, `wait=false` and cleanup behaviour; tests | `internal/install/image_lock.go`, `image_error.go` (new), `image.go`, tests | ✅ |
| 5a | Raise the default `maxDownloadBytes` from 2 GiB to 8 GiB for big ISOs | `internal/config/config.go`, `config.json`, `internal/download/downloader.go`, `README.md` | ✅ |
| 5b | ~~Download: accept image MIME types in the `Accept` header~~ Not needed: the header already ends with `*/*` | — | ✅ |
| 6 | GUI: installer type and image state in the details panel, unverified/installable/invalid states, invalid images blocked, the three Czech messages; state kept outside the GUI; tests and Wine screenshots | `internal/install/image_status.go` (new), `image.go`, `installer.go`, `internal/ui/app.go`, `app_details.go`, `installation.go`, `main_window.go` | ✅ |
| 7 | Docs: README table for `installerType`, the autorun.inf requirement; ISO example in `apps_example.json` | `README.md`, `apps_example.json` | ✅ |
| 8 | Admin panel (postponed by Jandy, do later): allow `.iso`/`.img` uploads, `iso`/`img` types, `.iso`/`.img` file names | `admin_panel_web/ProvisionerJsonCRUD.php`, `api.php`, `ProvisionerChunkUpload.php`, `index.php`, `assets/admin.js` | ⬜ |
| 9 | Build with `build.ps1`, test on real Windows with an ISO and an IMG, both EXE and MSI inside | — | ⬜ needs a real Windows 10/11 machine |
| 10 | Final verification: tests for every scenario, gofmt/vet/build, user-friendly fallback texts for mount failures | `internal/install/image_test.go`, `image_status.go` | ✅ |

## Verification (2026-10-14)

Tools: `gofmt -l .` clean; `go vet` clean for linux and windows; `go build -trimpath -ldflags "-s -w -H=windowsgui" ./cmd/winfly` passes for windows/amd64, 386 and arm64. `go test` passes on Linux for `config`, `download`, `diskimage`; the Windows-only packages (`install`, `diskimage`, `systeminfo`, all others) pass as Windows test binaries under Wine.

**Native mounting was NOT tested.** Wine's `virtdisk.dll` is a stub, so everything after "mount" ran against a fake mounter (a temp directory) or the fake OS layer of `diskimage`. Step 9 is still required.

| # | Scenario | How it was tested | Result |
|---|---|---|---|
| 1 | EXE and MSI unchanged | unit tests (`commandFor`, exit codes, `wait`), real EXE run under Wine and in the Wine GUI batch | ✅ EXE; MSI only unit-tested (no real `msiexec` run) |
| 2 | Valid ISO with autorun launches its installer | `TestRunFromImage`: fake mount + real process (test binary as installer), arguments, working dir, 3010 | ✅ mock |
| 3 | IMG follows the same workflow | `TestRunFromImageIMGWithCzechPath` | ✅ mock |
| 4 | Image without autorun rejected | `install` and `diskimage` tests; Czech text `MessageNoAutorun` | ✅ mock |
| 5 | Missing, malformed, icon-only, unsafe (`..`, drive letter) targets rejected | `TestRunFromImageRejectsBadAutorun`, `autorun_test.go` table | ✅ mock |
| 6 | Corrupted and unsupported images | wrong SHA-256 stops before mount; `format_test.go` (hard disk, raw CD, too small); Wine GUI shows `MessageUnsupportedFormat` | ✅ unit + Wine GUI |
| 7 | Cancellation/failure leaves nothing mounted | `TestRunFromImageUnmountsAfterCancellation`, failing installer, autorun errors; `diskimage` tests for owned vs. foreign mounts | ✅ mock |
| 8 | Batch continues after a failure | Wine GUI: ISO mount failed, broken IMG rejected, EXE then installed (`/mnt/project-files/iso-gui/batch-wine-*.png`) | ✅ Wine |
| 9 | Spaces and Czech characters in paths | `Účetnictví a kancelář/Kancelář 2024.IMG`, args `/jazyk čeština` | ✅ mock |
| 10 | DXDiag, Power Options, Domain/Workgroup intact | no diff in `systeminfo`, `systemdialogs`, `sysinfo_*`, `ui.go`; `main_window.go` one-line change only; buttons still visible in the Wine GUI | ✅ |

Fixed during verification: a mount failure without a fixed message showed the raw Windows error in the dialog; it now shows "Obraz disku se nepodařilo připojit. Podrobnosti najdete v logu." (details stay in the log).

Remaining platform limitations:

- Real `virtdisk.dll` mounting, drive-letter lookup and `SeManageVolumePrivilege` are untested (step 9).
- An autorun target that starts another process and exits at once loses the image when WinFly unmounts it; such installers need their own wait switch in `arguments`.
- The image file stays locked with `FILE_SHARE_READ` while mounted; that `AttachVirtualDisk` still opens it must be confirmed on real Windows.
- MSI inside an image runs through the same `msiexec /i` path, verified only by unit tests.

## Open questions

- Defaults kept until Jandy decides: an image or MSI with `wait: false` makes the whole list invalid (same as MSI); `.bat`/`.cmd`/`.hta` autorun targets stay rejected; no "Znovu ověřit" button (invalid images are retried after restart or a changed list entry); raising the 900 s download timeout to 3600 s is recommended but not done.
- Step 8 is postponed: until the panel accepts `.iso`/`.img` uploads and
  file names, ISO apps can only be added by editing `apps.json` by hand.
- `network.timeoutSeconds` (default 900 s) limits the whole download, so an
  8 GiB image needs roughly 9.5 MB/s to finish in time.
- `lockAndVerifyInstaller` opens the image with `FILE_SHARE_READ` only;
  `AttachVirtualDisk` must still be able to open it read-only. Verify in step 9 on real Windows.
- `auto` with an unknown extension (e.g. the panel's `app.bin`) still loads,
  exactly as before, and fails only when installed.
