"""Execute the actual npm publishing step with offline shell substitutes."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


BASH = shutil.which('bash')
WORKFLOW = Path(__file__).resolve().parents[1] / '.github/workflows/release.yml'


def publish_script():
    lines = WORKFLOW.read_text(encoding='utf-8').splitlines()
    step = lines.index('      - name: Publish to npm')
    start = lines.index('        run: |', step) + 1
    script = []
    for line in lines[start:]:
        if line and not line.startswith('          '):
            break
        script.append(line[10:])
    return '\n'.join(script)


@unittest.skipUnless(BASH, 'bash is required to exercise the publishing step')
class ReleaseWorkflowTests(unittest.TestCase):
    def run_publish(self, visible_on=1, publish_status=0, view_seconds=0):
        # Functions shadow every external command in the step. No npm process,
        # registry access, executable permissions or actual waits are involved.
        stubs = r'''
view_count=0
SECONDS=0
npm() {
  printf '%s\n' "$*" >> calls.log
  case "$1" in
    view)
      view_count=$((view_count + 1))
      SECONDS=$((SECONDS + VIEW_SECONDS))
      if [ "$view_count" -ge "$VISIBLE_ON" ]; then
        echo '1.2.3'
        return 0
      fi
      return 1
      ;;
    publish) return "$PUBLISH_STATUS" ;;
    *) return 99 ;;
  esac
}
sleep() { printf 'sleep %s\n' "$*" >> calls.log; SECONDS=$((SECONDS + $1)); }
'''
        with tempfile.TemporaryDirectory() as directory:
            result = subprocess.run(
                [BASH, '--noprofile', '--norc', '-eo', 'pipefail', '-c',
                 stubs + '\n' + publish_script()],
                cwd=directory, capture_output=True, text=True, encoding='utf-8', timeout=10,
                env={**os.environ, 'GITHUB_REF_NAME': 'v1.2.3',
                     'GITHUB_STEP_SUMMARY': 'summary.md',
                     'VISIBLE_ON': str(visible_on),
                     'VIEW_SECONDS': str(view_seconds),
                     'PUBLISH_STATUS': str(publish_status)},
            )
            calls = (Path(directory) / 'calls.log').read_text().splitlines()
            summary = Path(directory) / 'summary.md'
            return result, calls, summary.read_text(encoding='utf-8') if summary.exists() else ''

    def test_already_published_skips_upload(self):
        result, calls, _ = self.run_publish()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(calls), 1)
        self.assertTrue(calls[0].startswith('view shimo-net-cli@1.2.3 version '))
        for option in ['--registry=https://registry.npmjs.org/', '--prefer-online',
                       '--fetch-retries=0', '--fetch-timeout=15000']:
            self.assertIn(option, calls[0])

    def test_published_version_is_verified(self):
        result, calls, _ = self.run_publish(visible_on=2)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('publish ./dist/shimo-net-cli-1.2.3.tgz --access public', calls)
        self.assertEqual(sum(call.startswith('view ') for call in calls), 2)

    def test_registry_delay_is_retried(self):
        result, calls, _ = self.run_publish(visible_on=4)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls.count('sleep 15'), 2)
        self.assertEqual(sum(call.startswith('publish ') for call in calls), 1)

    def test_processing_beyond_three_minutes_can_finish(self):
        result, calls, _ = self.run_publish(visible_on=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(sum(call.startswith('publish ') for call in calls), 1)
        self.assertEqual(sum(call.startswith('view ') for call in calls), 30)

    def test_slow_registry_requests_count_toward_deadline(self):
        result, calls, _ = self.run_publish(visible_on=1000, view_seconds=15)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(sum(call.startswith('view ') for call in calls), 31)
        self.assertEqual(calls.count('sleep 15'), 30)
        self.assertIn('900s', result.stdout)

    def test_failed_publish_fails_the_job(self):
        result, calls, summary = self.run_publish(visible_on=100, publish_status=1)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('::error::', result.stdout)
        self.assertIn('1.2.3', summary)
        self.assertEqual(sum(call.startswith('view ') for call in calls), 1)

    def test_registry_visibility_timeout_fails_the_job(self):
        result, calls, _ = self.run_publish(visible_on=1000)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('::error::', result.stdout)
        self.assertEqual(sum(call.startswith('view ') for call in calls), 61)
        self.assertEqual(calls.count('sleep 15'), 60)


if __name__ == '__main__':
    unittest.main()
