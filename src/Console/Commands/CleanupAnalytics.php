<?php

namespace Webbesoft\Doorman\Console\Commands;

use Illuminate\Console\Command;
use Webbesoft\Doorman\Models\UserAnalytic;

class CleanupAnalytics extends Command
{
    protected $signature = 'analytics:cleanup 
                            {--days= : Number of days to retain (overrides config)}
                            {--dry-run : Show what would be deleted without actually deleting}';

    protected $description = 'Clean up old analytics data based on retention policy';

    public function handle(): int
    {
        $retentionDays = $this->option('days') ?? config('doorman.retention_days');

        if (! $retentionDays) {
            $this->info('No retention period configured. Skipping cleanup.');

            return self::SUCCESS;
        }

        $cutoffDate = now()->subDays($retentionDays)->format('Y-m-d');

        $query = UserAnalytic::where('date', '<', $cutoffDate);
        $count = $query->count();

        if ($count === 0) {
            $this->info('No old analytics data found to clean up.');

            return self::SUCCESS;
        }

        if ($this->option('dry-run')) {
            $this->info("Would delete {$count} analytics records older than {$cutoffDate}");

            return self::SUCCESS;
        }

        $this->info("Cleaning up analytics data older than {$cutoffDate}...");

        if ($this->confirm("This will delete {$count} analytics records. Continue?")) {
            $deletedCount = $query->delete();
            $this->info("Successfully deleted {$deletedCount} old analytics records.");
        } else {
            $this->info('Cleanup cancelled.');
        }

        return self::SUCCESS;
    }
}
