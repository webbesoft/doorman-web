<?php

namespace Webbesoft\Doorman;

use Illuminate\Support\ServiceProvider;
use Webbesoft\Doorman\Console\Commands\CleanupAnalytics;
use Webbesoft\Doorman\Services\AnalyticsService;

class AnalyticsServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        $this->mergeConfigFrom(
            __DIR__.'/../config/doorman.php',
            'doorman'
        );

        $this->app->singleton(AnalyticsService::class);
    }

    public function boot(): void
    {
        $this->publishConfig();
        $this->publishMigrations();
        $this->registerCommands();
    }

    protected function publishConfig(): void
    {
        $this->publishes([
            __DIR__.'/../config/doorman.php' => config_path('doorman.php'),
        ], 'doorman-config');
    }

    protected function publishMigrations(): void
    {
        $this->publishes([
            __DIR__.'/../database/migrations/' => database_path('migrations'),
        ], 'doorman-migrations');

        $this->loadMigrationsFrom(__DIR__.'/../database/migrations');
    }

    protected function registerCommands(): void
    {
        if ($this->app->runningInConsole()) {
            $this->commands([
                CleanupAnalytics::class,
            ]);
        }
    }
}
