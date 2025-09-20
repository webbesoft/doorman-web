<?php

namespace webbesoft\doorman\src\Filament\Widgets;

use Filament\Widgets\ChartWidget;
use webbesoft\doorman\Services\AnalyticsService;

class AnalyticsChartWidget extends ChartWidget
{
    protected ?string $heading = 'Daily Unique Visitors - Last 30 Days';

    protected static ?int $sort = 2;

    protected function getData(): array
    {
        $analyticsService = app(AnalyticsService::class);
        $stats = $analyticsService->getStats(
            now()->subDays(29),
            now()
        );

        $dates = [];
        $visitorData = [];

        // Generate all dates for the last 30 days
        for ($i = 29; $i >= 0; $i--) {
            $date = now()->subDays($i)->format('Y-m-d');
            $dates[] = now()->subDays($i)->format('M j');
            $visitorData[] = $stats['daily_stats'][$date] ?? 0;
        }

        return [
            'datasets' => [
                [
                    'label' => 'Unique Visitors',
                    'data' => $visitorData,
                    'backgroundColor' => 'rgba(59, 130, 246, 0.1)',
                    'borderColor' => 'rgb(59, 130, 246)',
                    'borderWidth' => 2,
                    'fill' => true,
                    'tension' => 0.1,
                ],
            ],
            'labels' => $dates,
        ];
    }

    protected function getType(): string
    {
        return 'line';
    }

    protected function getOptions(): array
    {
        return [
            'scales' => [
                'y' => [
                    'beginAtZero' => true,
                    'ticks' => [
                        'stepSize' => 1,
                    ],
                ],
            ],
            'plugins' => [
                'legend' => [
                    'display' => true,
                ],
            ],
            'maintainAspectRatio' => false,
        ];
    }
}
