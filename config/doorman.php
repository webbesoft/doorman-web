<?php

return [
    /*
    |--------------------------------------------------------------------------
    | Doorman Configuration
    |--------------------------------------------------------------------------
    */

    'enabled' => env('DOORMAN_ENABLED', true),

    /*
    |--------------------------------------------------------------------------
    | Route Exclusions
    |--------------------------------------------------------------------------
    |
    | Patterns of routes to exclude from tracking.
    |
    */
    'exclude_routes' => [
        'api/*',
        'admin/*',
        '_debugbar/*',
        'telescope/*',
        'horizon/*',
        'nova/*',
        'nova-api/*',
    ],

    /*
    |--------------------------------------------------------------------------
    | Bot Detection
    |--------------------------------------------------------------------------
    |
    | User agents containing these strings will be excluded from tracking.
    |
    */
    'bot_patterns' => [
        'bot', 'crawler', 'spider', 'scraper', 'parser',
        'googlebot', 'bingbot', 'slurp', 'duckduckbot',
        'facebookexternalhit', 'twitterbot', 'whatsapp',
        'lighthouse', 'pagespeed', 'pingdom', 'uptimerobot',
        'headlesschrome', 'phantom', 'selenium',
    ],

    /*
    |--------------------------------------------------------------------------
    | Data Retention
    |--------------------------------------------------------------------------
    |
    | How long to keep analytics data (in days).
    | Set to null to keep data forever.
    |
    */
    'retention_days' => env('DOORMAN_RETENTION_DAYS', 365),

    /*
    |--------------------------------------------------------------------------
    | Database Table Name
    |--------------------------------------------------------------------------
    |
    | The name of the database table to store analytics data.
    |
    */
    'table_name' => 'analytics',
];
