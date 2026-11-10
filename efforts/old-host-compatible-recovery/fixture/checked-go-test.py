"""Run one Go package and require explicit, unskipped intended test execution."""
import json
import subprocess
import sys


def main():
    if len(sys.argv) < 5:
        print("usage: checked-go-test.py PACKAGE PATTERN TIMEOUT EXPECTED_TEST...",
              file=sys.stderr)
        return 2
    package, pattern, timeout, *expected = sys.argv[1:]
    # Keep Go's failure status authoritative; package PASS alone is not evidence
    # of intended test execution. No shell pipeline can hide go test's exit code.
    result = subprocess.run(
        ['go', 'test', package, '-run', pattern, '-count=1', '-json',
         '-timeout=' + timeout], stdout=subprocess.PIPE, text=True)
    print(result.stdout, end="", flush=True)
    if result.returncode:
        return result.returncode if result.returncode > 0 else 1
    ran, passed = set(), set()
    rejected = []
    try:
        for line in result.stdout.splitlines():
            event = json.loads(line)
            name, action = event.get('Test'), event.get('Action')
            if action in ('skip', 'fail'):
                rejected.append(name or event.get('Package', '<package>'))
            if action == 'run' and name:
                ran.add(name)
            if action == 'pass' and name:
                passed.add(name)
    except (ValueError, AttributeError):
        print('Invalid go test JSON stream; refusing acceptance', file=sys.stderr)
        return 1
    missing = set(expected) - (ran & passed)
    if missing or rejected:
        print('Intended tests did not all execute and pass: missing=' +
              repr(sorted(missing)) + ', skipped/failed=' + repr(rejected),
              file=sys.stderr)
        return 1
    print(f'Verified {len(set(expected))} intended tests ran and passed (zero skips)')
    return 0


if __name__ == '__main__':
    sys.exit(main())
