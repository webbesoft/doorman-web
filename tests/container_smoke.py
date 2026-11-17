#!/usr/bin/env python3
"""Exercise only an isolated image, temporary containers, and a temporary Docker volume."""
import http.cookies
import json
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

image = sys.argv[1] if len(sys.argv) > 1 else 'doorman-check:local'
name = 'doorman-smoke-' + uuid.uuid4().hex[:12]
volume = name + '-data'
restore_volume = name + '-restore'
fixture_env = ['-e', 'APP_ENV=production', '-e', 'DOORMAN_ADMIN_USER=smoke-owner',
               '-e', 'DOORMAN_ADMIN_PASSWORD=smoke-test-password-only',
               '-e', 'DOORMAN_SESSION_SECRET=0123456789abcdef0123456789abcdef',
               '-e', 'DOORMAN_ALLOWED_HOSTS=example.com', '-e', 'DB_PATH=/data/analytics.db']

def docker(*args):
    return subprocess.check_output(['docker', *args], text=True).strip()

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None

opener = urllib.request.build_opener(NoRedirect())

def request(base, path, data=None, headers=None):
    req = urllib.request.Request(base + path, data=data, headers=headers or {})
    try:
        response = opener.open(req, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, response.headers, response.read().decode()

def start(data_volume=volume):
    docker('run', '-d', '--rm', '--name', name, '-p', '127.0.0.1::8080', '-v', data_volume + ':/data', '--read-only', '--tmpfs', '/tmp', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', *fixture_env, image)
    port = docker('port', name, '8080/tcp').rsplit(':', 1)[1]
    base = 'http://127.0.0.1:' + port
    for _ in range(150):
        try:
            if request(base, '/readyz')[0] == 200:
                return base
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(0.2)
    raise AssertionError('Container did not become ready')

def login(base):
    form = urllib.parse.urlencode({'username': 'smoke-owner', 'password': 'smoke-test-password-only'}).encode()
    status, headers, _ = request(base, '/login', form, {'Content-Type': 'application/x-www-form-urlencoded'})
    assert status == 303, 'Login failed'
    cookie = http.cookies.SimpleCookie(headers['Set-Cookie'])
    return '; '.join(f'{key}={value.value}' for key, value in cookie.items())

try:
    docker('volume', 'create', volume)
    docker('run', '--rm', '-v', volume + ':/data', *fixture_env, image, '-migrate')
    base = start()
    assert docker('exec', name, 'id', '-u') != '0', 'Container runs as root'
    payload = json.dumps({'visitId': uuid.uuid4().hex, 'sequence': 1, 'url': 'https://example.com/smoke',
                          'dwellTime': 5, 'activeTime': 3, 'scrollDepth': 50}).encode()
    for _ in range(2):
        assert request(base, '/event', payload, {'Content-Type': 'text/plain', 'Origin': 'https://example.com', 'User-Agent': 'Mozilla/5.0'})[0] == 204
    cookie = login(base)
    status, _, body = request(base, '/dashboard', headers={'Cookie': cookie})
    assert status == 200 and 'https://example.com/smoke' in body
    assert re.search(r'class="metric primary-metric".*?<strong>1</strong>', body, re.S), 'Duplicate delivery inflated visits'
    for asset in ['/assets/js/t.js', '/assets/js/dashboard.js', '/assets/css/doorman.css']:
        assert request(base, asset)[0] == 200, 'Packaged asset missing: ' + asset
    docker('stop', '--time', '15', name)
    base = start()
    cookie = login(base)
    assert 'https://example.com/smoke' in request(base, '/dashboard', headers={'Cookie': cookie})[2], 'Replacement container lost data'
    docker('exec', name, '/app/doorman', '-backup', '/data/verified-backup.db')
    docker('exec', name, '/app/doorman', '-verify-backup', '/data/verified-backup.db')
    docker('stop', '--time', '15', name)
    docker('volume', 'create', restore_volume)
    docker('run', '--rm', '--entrypoint', '/bin/sh',
           '-v', volume + ':/backup:ro', '-v', restore_volume + ':/data', image,
           '-c', 'cp /backup/verified-backup.db /data/analytics.db')
    base = start(restore_volume)
    cookie = login(base)
    status, _, body = request(base, '/dashboard', headers={'Cookie': cookie})
    assert status == 200 and 'https://example.com/smoke' in body, 'Restored backup lost the smoke event'
    assert re.search(r'class="metric primary-metric".*?<strong>1</strong>', body, re.S), 'Restored visit count differs'
    assert request(base, '/logout', b'', {'Cookie': cookie})[0] == 303
    assert request(base, '/dashboard', headers={'Cookie': cookie})[0] == 303, 'Logged-out cookie replayed'
    print('Container smoke passed: non-root, packaged assets, migrations, idempotency, replacement persistence, backup verification, restore, logout')
finally:
    subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(['docker', 'volume', 'rm', volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(['docker', 'volume', 'rm', restore_volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
