<?php

namespace webbesoft\doorman\Models;

use Carbon\Carbon;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\Model;

class UserAnalytic extends Model
{
    protected $table = 'user_analytics';

    protected $fillable = [
        'identifier',
        'identifier_type',
        'date',
        'page',
    ];

    protected $casts = [
        'date' => 'datetime:Y-m-d H:i:s',
    ];

    // Scopes
    public function scopeByDate(Builder $query, Carbon $date): Builder
    {
        return $query->where('date', $date->format('Y-m-d'));
    }

    public function scopeByDateRange(Builder $query, Carbon $start, Carbon $end): Builder
    {
        return $query->whereBetween('date', [
            $start->format('Y-m-d'),
            $end->format('Y-m-d'),
        ]);
    }

    public function scopeByType(Builder $query, string $type): Builder
    {
        return $query->where('identifier_type', $type);
    }

    public function scopeToday(Builder $query): Builder
    {
        return $query->byDate(now());
    }

    public function scopeThisWeek(Builder $query): Builder
    {
        return $query->byDateRange(now()->startOfWeek(), now()->endOfWeek());
    }

    public function scopeThisMonth(Builder $query): Builder
    {
        return $query->byDateRange(now()->startOfMonth(), now()->endOfMonth());
    }

    // Static helper methods
    public static function getUniqueVisitorsForDate(Carbon $date): int
    {
        return static::byDate($date)->count();
    }

    public static function getUniqueVisitorsForPeriod(Carbon $start, Carbon $end): int
    {
        return static::byDateRange($start, $end)->count();
    }

    public static function getDailyStats(Carbon $start, Carbon $end): array
    {
        return static::selectRaw('date, COUNT(*) as unique_visitors')
            ->byDateRange($start, $end)
            ->groupBy('date')
            ->orderBy('date')
            ->get()
            ->mapWithKeys(fn ($item) => [$item->date->format('Y-m-d') => $item->unique_visitors])
            ->toArray();
    }

    public static function getTypeBreakdown(Carbon $start, Carbon $end): array
    {
        return static::selectRaw('identifier_type, COUNT(*) as count')
            ->byDateRange($start, $end)
            ->groupBy('identifier_type')
            ->pluck('count', 'identifier_type')
            ->toArray();
    }
}
