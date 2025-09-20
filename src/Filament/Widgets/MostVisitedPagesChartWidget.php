<?php

namespace webbesoft\doorman\src\Filament\Widgets;

use Filament\Widgets\ChartWidget;
use webbesoft\doorman\Models\PageVisit;

class MostVisitedPagesChart extends ChartWidget
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
