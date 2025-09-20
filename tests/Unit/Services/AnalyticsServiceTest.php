<?php

namespace webbesoft\doorman\Tests\Unit;

beforeEach(function () {
    $this->service = new \webbesoft\doorman\Services\AnalyticsService;
});

it('tracks unique visitors', function () {});

it('obfuscates IP addresses', function () {});
it('gets unique identifier for authenticated users', function () {});
it('gets unique identifier for guest users with session', function () {});
it('gets unique identifier for guest users without session', function () {});
