<?php

namespace webbesoft\doorman;

use Illuminate\Support\ServiceProvider;
use webbesoft\doorman\Console\Commands\CleanupAnalytics;
use webbesoft\doorman\Services\AnalyticsService;

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
            __DIR__.'/../config/simple-analytics.php' => config_path('simple-analytics.php'),
        ], 'simple-analytics-config');
    }

    protected function publishMigrations(): void
    {
        $this->publishes([
            __DIR__.'/../database/migrations/' => database_path('migrations'),
        ], 'simple-analytics-migrations');

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
