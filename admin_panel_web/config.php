<?php

declare(strict_types=1);

return [
    // apps.json zustava ve stejne slozce jako administrace.
    'appsFile' => __DIR__ . DIRECTORY_SEPARATOR . 'apps.json',

    // Instalacni soubory se ukladaji o jednu uroven vyse nez administrace.
    'installerDirectory' => dirname(__DIR__),

    // Verejna URL odpovidajici nadrazene slozce s instalatory.
    'installerBaseUrl' => 'https://static.essensworld.com/helpdesk/',
];
