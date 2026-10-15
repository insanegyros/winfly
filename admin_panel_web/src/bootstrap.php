<?php

declare(strict_types=1);

// Loads the panel's code. Included by index.php and api.php.

const WINFLY_ADMIN = true;

require_once __DIR__ . '/ApiError.php';
require_once __DIR__ . '/files.php';
require_once __DIR__ . '/settings.php';
require_once __DIR__ . '/session.php';
require_once __DIR__ . '/http.php';
require_once __DIR__ . '/installer_types.php';
require_once __DIR__ . '/app_record.php';
require_once __DIR__ . '/AppRepository.php';
require_once __DIR__ . '/InstallerStorage.php';
require_once __DIR__ . '/ChunkUpload.php';
require_once __DIR__ . '/ApiActions.php';
