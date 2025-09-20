<?php

namespace Webbesoft\Doorman\Services;

use Carbon\Carbon;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Webbesoft\Doorman\Models\PageVisit;
use Webbesoft\Doorman\Models\UserAnalytic;

class AnalyticsService
{
    public function track(Request $request): void
    {
        $identifier = $this->getUniqueIdentifier($request);
        $today = now()->format('Y-m-d H:i:s');
        $page = $request->path();

        if ($identifier) {
            $this->recordVisit($identifier['value'], $identifier['type'], $today, $page);
        }
    }

    /**
     * Get the best unique identifier for the visitor
     * Priority: user_id > session_id > hashed_ip
     */
    protected function getUniqueIdentifier(Request $request): ?array
    {
        // Priority 1: Authenticated user
        if (Auth::check()) {
            return [
                'value' => (string) Auth::id(),
                'type' => 'user',
            ];
        }

        // Priority 2: Session ID (for guests with sessions)
        if ($request->hasSession() && $request->session()->getId()) {
            return [
                'value' => $request->session()->getId(),
                'type' => 'session',
            ];
        }

        // Priority 3: Obfuscated IP address
        if ($request->ip()) {
            return [
                'value' => $this->obfuscateIp($request->ip()),
                'type' => 'ip',
            ];
        }

        return null;
    }

    /**
     * Create a one-way hash of the IP address for privacy
     */
    protected function obfuscateIp(string $ip): string
    {
        $salt = config('app.key').now()->format('Y-m-d');

        return hash('sha256', $ip.$salt);
    }

    protected function recordVisit(string $identifier, string $type, string $date, string $page): void
    {
        UserAnalytic::updateOrCreate(
            [
                'identifier' => $identifier,
            ],
            [
                'identifier_type' => $type,
                'date' => $date,
                'page' => $page ?? '/',
            ]
        );

        PageVisit::create([
            'page' => $page ?? '/',
            'visited_at' => $date,
            'identifier' => $identifier,
            'identifier_type' => $type,
        ]);
    }

    public function getStats(?Carbon $start = null, ?Carbon $end = null): array
    {
        $start = $start ?: now()->subDays(30);
        $end = $end ?: now();

        return [
            'unique_visitors' => UserAnalytic::getUniqueVisitorsForPeriod($start, $end),
            'daily_stats' => UserAnalytic::getDailyStats($start, $end),
            'type_breakdown' => UserAnalytic::getTypeBreakdown($start, $end),
            'period' => [
                'start' => $start->format('Y-m-d'),
                'end' => $end->format('Y-m-d'),
            ],
        ];
    }

    public function getTodayStats(): array
    {
        $endOfDay = now()->endOfDay();
        $today = now()->startOfDay();
        $typeBreakdown = UserAnalytic::getTypeBreakdown($today, $endOfDay);

        return [
            'unique_visitors' => UserAnalytic::getUniqueVisitorsForDate($today),
            'authenticated_users' => $typeBreakdown['user'] ?? 0,
            'guest_sessions' => $typeBreakdown['session'] ?? 0,
            'unknown_visitors' => $typeBreakdown['ip'] ?? 0,
        ];
    }

    public function getWeeklyGrowth(): array
    {
        $thisWeek = UserAnalytic::thisWeek()->count();
        $lastWeek = UserAnalytic::byDateRange(
            now()->subWeek()->startOfWeek(),
            now()->subWeek()->endOfWeek()
        )->count();

        return [
            'this_week' => $thisWeek,
            'last_week' => $lastWeek,
            'growth_percentage' => $lastWeek > 0
                ? round((($thisWeek - $lastWeek) / $lastWeek) * 100, 1)
                : ($thisWeek > 0 ? 100 : 0),
        ];
    }
}
