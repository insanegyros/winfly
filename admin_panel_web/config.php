<?php

declare(strict_types=1);

return [
    // Heslo pro prihlaseni do administrace. Muzete ho zmenit na libovolne vlastni heslo.
    'adminPassword' => 'Essensadmin21+',

    // apps.json zustava ve stejne slozce jako administrace.
    'appsFile' => __DIR__ . DIRECTORY_SEPARATOR . 'apps.json',

    // Instalacni soubory se ukladaji o jednu uroven vyse nez administrace.
    'installerDirectory' => dirname(__DIR__),

    // Verejna URL odpovidajici nadrazene slozce s instalatory.
    'installerBaseUrl' => 'https://static.essensworld.com/helpdesk/',

    // Volitelne nastaveni blokoveho nahravani (vychozi hodnoty obvykle staci):
    // 'chunkUploadDirectory' => __DIR__ . DIRECTORY_SEPARATOR . '.winfly-uploads',
    // 'chunkSizeBytes' => 1048576,
    // 'chunkMinimumSizeBytes' => 65536,
    // 'chunkUploadMaxAgeSeconds' => 86400,
];
