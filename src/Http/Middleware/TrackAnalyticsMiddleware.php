<?php

namespace Webbesoft\Doorman\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Webbesoft\Doorman\Services\AnalyticsService;

class TrackAnalyticsMiddleware
{
    protected AnalyticsService $analyticsService;

    public function __construct(AnalyticsService $analyticsService)
    {
        $this->analyticsService = $analyticsService;
    }

    public function handle(Request $request, Closure $next)
    {
        $response = $next($request);

        if ($this->shouldTrack($request, $response)) {
            try {
                $this->analyticsService->track($request);
            } catch (\Exception $e) {
                // Silently fail - don't break the user experience
                report($e);
            }
        }

        return $response;
    }

    protected function shouldTrack(Request $request, $response): bool
    {
        // Check if analytics is enabled
        if (! config('doorman.enabled', true)) {
            return false;
        }

        // Only track successful GET requests
        if (! $request->isMethod('GET') || ! $response->isSuccessful()) {
            return false;
        }

        // Check route exclusions
        $excludeRoutes = config('doorman.exclude_routes', []);
        foreach ($excludeRoutes as $pattern) {
            if ($request->is($pattern)) {
                return false;
            }
        }

        // Check for bots
        if ($this->isBot($request)) {
            return false;
        }

        return true;
    }

    protected function isBot(Request $request): bool
    {
        $userAgent = $request->userAgent();

        if (! $userAgent) {
            return true;
        }

        $bots = config('doorman.bot_patterns', [
            'bot', 'crawler', 'spider', 'scraper', 'parser',
            'googlebot', 'bingbot', 'slurp', 'duckduckbot',
            'facebookexternalhit', 'twitterbot', 'whatsapp',
            'lighthouse', 'pagespeed', 'pingdom', 'uptimerobot',
        ]);

        foreach ($bots as $bot) {
            if (stripos($userAgent, $bot) !== false) {
                return true;
            }
        }

        return false;
    }
}
