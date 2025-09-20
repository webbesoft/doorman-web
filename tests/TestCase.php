<?php

declare(strict_types=1);

namespace Webbesoft\Doorman\Tests;

use Orchestra\Testbench\TestCase as Orchestra;

class TestCase extends Orchestra
{
    protected function setUp(): void
    {
        parent::setUp();

        // Additional setup if needed
    }

    protected function getPackageProviders($app)
    {
        return [
            \webbesoft\doorman\AnalyticsServiceProvider::class,
        ];
    }
}
