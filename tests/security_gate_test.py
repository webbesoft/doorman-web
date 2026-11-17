import json
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class SecurityGateTests(unittest.TestCase):
    def gate(self, advisory='GO-2026-6872', version='v4.16.5', *, findings=None, status=0, progress=True):
        entries = [{'config': {'scanner_name': 'govulncheck'}}]
        if progress:
            entries.append({'progress': {'message': 'Checking packages'}})
            if findings is None:
                findings = [(advisory, 'github.com/labstack/echo/v4', version)]
            for osv, module, version in findings:
                entries.append({'finding': {'osv': osv, 'trace': [{'module': module, 'version': version, 'function': 'New'}]}})
        stream = '\n'.join(json.dumps(entry) for entry in entries)
        with tempfile.TemporaryDirectory() as work:
            work = pathlib.Path(work)
            output = work / 'results.json'
            output.write_text(stream)
            go = work / 'go'
            go.write_text('#!/usr/bin/env python3\nimport os,sys\nsys.stdout.write(os.environ["TEST_SCANNER_OUTPUT"])\nsys.exit(int(os.environ["TEST_SCANNER_STATUS"]))\n')
            go.chmod(0o755)
            env = dict(os.environ, PATH=str(work) + os.pathsep + os.environ['PATH'],
                       TEST_SCANNER_OUTPUT=stream, TEST_SCANNER_STATUS=str(status))
            return subprocess.run(['python3', str(ROOT / 'scripts/check-vulnerabilities.py'), str(output)],
                                  env=env, capture_output=True).returncode

    def test_only_exact_reviewed_advisories_are_accepted(self):
        self.assertEqual(self.gate(), 0)
        self.assertNotEqual(self.gate(version='v4.99.0'), 0)
        self.assertNotEqual(self.gate(advisory='GO-9999-UNKNOWN'), 0)

    def test_distinct_module_versions_cannot_overwrite_each_other(self):
        entries = [('GO-2026-6872', 'github.com/labstack/echo/v5', 'v5.0.0'),
                   ('GO-2026-6872', 'github.com/labstack/echo/v4', 'v4.16.5')]
        for findings in [entries, entries[::-1]]:
            with self.subTest(findings=findings):
                self.assertNotEqual(self.gate(findings=findings), 0)

    def test_scanner_failures_and_config_only_output_cannot_pass(self):
        self.assertNotEqual(self.gate(status=7), 0)
        self.assertNotEqual(self.gate(status=7, progress=False), 0)
        self.assertNotEqual(self.gate(progress=False), 0)
