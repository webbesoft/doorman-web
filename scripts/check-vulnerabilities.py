#!/usr/bin/env python3
"""Fail on symbol-level findings unless an exact module version was reviewed."""
import json
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
exceptions = json.loads((root / 'security/vulnerability-exceptions.json').read_text())
output = pathlib.Path(sys.argv[1])
with output.open('w') as stream:
    result = subprocess.run(['go', 'tool', 'govulncheck', '-json', './...'], cwd=root, stdout=stream)
if result.returncode != 0:
    raise SystemExit(result.returncode)
raw = output.read_text()
decoder = json.JSONDecoder()
findings = {}
messages = 0
progress = False
while raw.strip():
    raw = raw.lstrip()
    message, length = decoder.raw_decode(raw)
    raw = raw[length:]
    messages += 1
    progress = progress or 'progress' in message
    finding = message.get('finding', {})
    trace = finding.get('trace', [])
    if trace and trace[0].get('function'):
        findings[(finding['osv'], trace[0].get('module'), trace[0].get('version'))] = trace[0]
if messages == 0:
    raise SystemExit('Scanner returned no results')
if not progress:
    raise SystemExit('Scanner returned no analysis progress')
failed = []
for (advisory, module, version), trace in sorted(findings.items()):
    exception = exceptions.get(advisory, {})
    if exception and exception.get('module') == trace.get('module') and exception.get('version') == trace.get('version'):
        print(f'{advisory}: reviewed as not affected for {trace["module"]}@{trace["version"]}')
    else:
        failed.append(f'{advisory} ({module}@{version})')
if failed:
    raise SystemExit('Unreviewed vulnerabilities: ' + ', '.join(failed))
print(f'Security gate passed; {len(findings)} precisely scoped advisory decisions')
