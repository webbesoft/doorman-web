import json
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class ReleaseTests(unittest.TestCase):
    def release(self, smoke_status=0):
        with tempfile.TemporaryDirectory() as work:
            work = pathlib.Path(work)
            (work / 'tests').mkdir()
            log = work / 'commands.jsonl'
            docker = work / 'docker'
            docker.write_text('''#!/usr/bin/env python3
import json,os,pathlib,sys
args=sys.argv[1:]
with open(os.environ['TEST_COMMAND_LOG'],'a') as log: log.write(json.dumps(['docker',*args])+'\\n')
if args[0]=='build':
 pathlib.Path(args[args.index('--iidfile')+1]).write_text('sha256:built-by-this-invocation')
''')
            docker.chmod(0o755)
            (work / 'tests/container_smoke.py').write_text('''import json,os,sys
with open(os.environ['TEST_COMMAND_LOG'],'a') as log: log.write(json.dumps(['smoke',*sys.argv[1:]])+'\\n')
sys.exit(int(os.environ['TEST_SMOKE_STATUS']))
''')
            env = dict(os.environ, PATH=str(work) + os.pathsep + os.environ['PATH'],
                       TEST_COMMAND_LOG=str(log), TEST_SMOKE_STATUS=str(smoke_status))
            result = subprocess.run(['make', '-f', str(ROOT / 'Makefile'), '-o', 'check', '-o', 'security',
                                     'release', 'VERSION=review-test'], cwd=work, env=env, capture_output=True)
            commands = [json.loads(line) for line in log.read_text().splitlines()]
            return result.returncode, commands

    def test_smoke_and_tag_use_the_id_captured_by_this_build(self):
        status, commands = self.release()
        self.assertEqual(status, 0)
        self.assertIn('--iidfile', commands[0])
        self.assertEqual(commands[1], ['smoke', 'sha256:built-by-this-invocation'])
        self.assertEqual(commands[2], ['docker', 'tag', 'sha256:built-by-this-invocation',
                                     'ghcr.io/webbesoft/doorman-web:review-test'])
        self.assertEqual(commands[3][0:2], ['docker', 'push'])
        self.assertFalse(pathlib.Path(commands[0][commands[0].index('--iidfile') + 1]).exists())

    def test_failed_smoke_does_not_tag_or_publish(self):
        status, commands = self.release(smoke_status=7)
        self.assertNotEqual(status, 0)
        self.assertEqual(len(commands), 2)
