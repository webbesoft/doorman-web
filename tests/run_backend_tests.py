#!/usr/bin/env python3
"""Run backend contracts against disposable local Docker databases."""
import os
import subprocess
import time
import uuid

names = []
environment = dict(os.environ)
environment['GOCACHE'] = '/tmp/doorman-hardening-go-cache'

def docker(*args):
    return subprocess.check_output(['docker', *args], text=True).strip()

try:
    for provider, image, port in [('postgres', 'postgres:18-alpine', 5432), ('mysql', 'mysql:8.4', 3306)]:
        name = 'doorman-contract-' + provider + '-' + uuid.uuid4().hex[:10]
        names.append(name)
        if provider == 'postgres':
            args = ['-e', 'POSTGRES_USER=doorman', '-e', 'POSTGRES_PASSWORD=contract-fixture-password', '-e', 'POSTGRES_DB=doorman_test']
        else:
            args = ['-e', 'MYSQL_ROOT_PASSWORD=contract-fixture-password', '-e', 'MYSQL_DATABASE=doorman_test', '-e', 'MYSQL_USER=doorman', '-e', 'MYSQL_PASSWORD=contract-fixture-password']
        docker('run', '-d', '--rm', '--name', name, '-p', f'127.0.0.1::{port}', *args, image)
        for _ in range(240):
            command = ['pg_isready', '-h', '127.0.0.1', '-U', 'doorman', '-d', 'doorman_test'] if provider == 'postgres' else ['mysqladmin', 'ping', '-h', '127.0.0.1', '--silent']
            if subprocess.run(['docker', 'exec', name, *command], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
                break
            time.sleep(0.25)
        else:
            raise RuntimeError(provider + ' did not become ready')
        assigned = docker('port', name, f'{port}/tcp').rsplit(':', 1)[1]
        dsn = f'postgres://doorman:contract-fixture-password@127.0.0.1:{assigned}/doorman_test?sslmode=disable' if provider == 'postgres' else f'doorman:contract-fixture-password@tcp(127.0.0.1:{assigned})/doorman_test?parseTime=true&tls=false'
        environment['DOORMAN_TEST_' + provider.upper() + '_DSN'] = dsn
    subprocess.run(['go', 'test', '-race', '-count=1', '-run', '^TestBackendContracts$', '-v', './internal/database'], env=environment, check=True)
finally:
    for name in names:
        subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
