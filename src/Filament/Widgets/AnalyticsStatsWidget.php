<?php

namespace webbesoft\doorman\Filament\Widgets;

use Filament\Widgets\StatsOverviewWidget as BaseWidget;
use Filament\Widgets\StatsOverviewWidget\Stat;
use webbesoft\doorman\Services\AnalyticsService;

class AnalyticsStatsWidget extends BaseWidget
{
    protected static ?int $sort = 1;

    protected function getStats(): array
    {
        $analyticsService = app(AnalyticsService::class);
        $todayStats = $analyticsService->getTodayStats();
        $weeklyGrowth = $analyticsService->getWeeklyGrowth();

        return [
            Stat::make('Unique Visitors Today', $todayStats['unique_visitors'])
                ->description($this->getGrowthDescription($weeklyGrowth))
                ->descriptionIcon($this->getGrowthIcon($weeklyGrowth))
                ->color($this->getGrowthColor($weeklyGrowth)),

            Stat::make('Authenticated Users', $todayStats['authenticated_users'])
                ->description('Logged in users today')
                ->descriptionIcon('heroicon-m-user')
                ->color('success'),

            Stat::make('Guest Visitors', $todayStats['guest_sessions'] + $todayStats['unknown_visitors'])
                ->description('Anonymous visitors today')
                ->descriptionIcon('heroicon-m-user-group')
                ->color('primary'),
        ];
    }

    protected function getGrowthDescription(array $growth): string
    {
        $percentage = $growth['growth_percentage'];

        if ($percentage == 0) {
            return 'No change from last week';
        }

        $direction = $percentage > 0 ? 'increase' : 'decrease';

        return sprintf('%.1f%% %s from last week', abs($percentage), $direction);
    }

    protected function getGrowthIcon(array $growth): string
    {
        $percentage = $growth['growth_percentage'];

        if ($percentage > 0) {
            return 'heroicon-m-arrow-trending-up';
        } elseif ($percentage < 0) {
            return 'heroicon-m-arrow-trending-down';
        }

        return 'heroicon-m-minus';
    }

    protected function getGrowthColor(array $growth): string
    {
        $percentage = $growth['growth_percentage'];

        if ($percentage > 0) {
            return 'success';
        } elseif ($percentage < 0) {
            return 'danger';
        }

        return 'gray';
    }
}
