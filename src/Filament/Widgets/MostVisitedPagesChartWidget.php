<?php

namespace Webbesoft\Doorman\Filament\Widgets;

use Filament\Widgets\ChartWidget;
use Webbesoft\Doorman\Models\PageVisit;

class MostVisitedPagesChartWidget extends ChartWidget
{
    protected static ?string $heading = 'Most Visited Pages';

    protected function getData(): array
    {
        $pages = PageVisit::select('page')
            ->selectRaw('COUNT(*) as total')
            ->groupBy('page')
            ->orderByDesc('total')
            ->limit(5)
            ->get();

        return [
            'datasets' => [
                [
                    'label' => 'Visits',
                    'data' => $pages->pluck('total')->toArray(),
                ],
            ],
            'labels' => $pages->pluck('page')->toArray(),
        ];
    }

    protected function getType(): string
    {
        return 'bar';
    }
}
