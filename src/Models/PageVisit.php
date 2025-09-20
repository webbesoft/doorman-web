<?php

namespace webbesoft\doorman\Models;

use Illuminate\Database\Eloquent\Model;

class PageVisit extends Model
{
    protected $table = 'doorman_page_visits';

    protected $fillable = [
        'identifier',
        'identifier_type',
        'visited_at',
        'page',
    ];

    public function casts(): array
    {
        return [
            'visited_at' => 'datetime:Y-m-d H:i:s',
        ];
    }

    public function scopeByDate($query, $date)
    {
        return $query->whereDate('visited_at', $date->format('Y-m-d'));
    }

    public function scopeByPage($query, $page)
    {
        return $query->where('page', $page);
    }

    public function scopeByType($query, $type)
    {
        return $query->where('identifier_type', $type);
    }

    public function scopeToday($query)
    {
        return $query->byDate(now());
    }

    public function scopeThisWeek($query)
    {
        return $query->byDateRange(now()->startOfWeek(), now()->endOfWeek());
    }

    public function scopeThisMonth($query)
    {
        return $query->byDateRange(now()->startOfMonth(), now()->endOfMonth());
    }

    public function scopeByDateRange($query, $start, $end)
    {
        return $query->whereBetween('visited_at', [
            $start->format('Y-m-d'),
            $end->format('Y-m-d'),
        ]);
    }

    public static function getUniqueVisitorsForDate($date): int
    {
        return static::byDate($date)->distinct('identifier')->count('identifier');
    }

    public static function getUniqueVisitorsForPeriod($start, $end): int
    {
        return static::byDateRange($start, $end)->distinct('identifier')->count('identifier');
    }

    public static function getDailyStats($start, $end): array
    {
        $stats = [];
        $currentDate = $start->copy();

        while ($currentDate->lte($end)) {
            $dateKey = $currentDate->format('Y-m-d');
            $stats[$dateKey] = static::getUniqueVisitorsForDate($currentDate);
            $currentDate->addDay();
        }

        return $stats;
    }
}
