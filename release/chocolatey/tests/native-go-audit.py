"""Offline audit of this bounded, incomplete synthetic run; never contacts a VM."""
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[3]
out = root / 'efforts/cross-platform-updates'
evidence = json.loads((out / 'native-go-evidence.json').read_text())
build = json.loads((out / 'native-go-build.json').read_text())
def content(name):
    return next(x['content'] for x in evidence if x['path'].endswith('\\' + name))
def parsed(name):
    return json.loads(content(name))
commands = parsed('native-go-commands.json')
launches = parsed('native-go-results.json')
snapshot = parsed('closed-before-release-snapshot.json')
log = content('closed-upgrade-held.log').splitlines()
checks = []
def check(label, status, detail):
    checks.append(dict(label=label, status=status, detail=detail))
check('fixture-source-identity', 'PASS' if hashlib.sha256(build['source'].encode()).hexdigest() == build['source_sha256'] else 'FAIL', build['source_sha256'])
for launch in launches:
    expected = 'native-go-fixture version=1.0.0 resource=resource-1.0.0'
    check(launch['label'], 'PASS' if launch['exit'] == 0 and launch['output'] == [expected] else 'FAIL', launch)
for file in snapshot['files']:
    if file['path'].endswith('\\tools\\resource.txt') or file['path'].endswith('\\tools\\cercano-native-go-20261106201208-closed.exe'):
        check('closed-B-bytes:' + file['path'], 'PASS' if any(x is not None and 'closed/2.0.0/' in x for x in file['knownSourceMatches']) else 'FAIL', file)
deletes = [i+1 for i, line in enumerate(log) if 'Attempting to delete file' in line and ('\\tools\\resource.txt' in line or '\\tools\\cercano-native-go-20261106201208-closed.exe' in line)]
scripts = [i+1 for i, line in enumerate(log) if 'Running package script' in line]
check('old-file-removal-attempts-before-B-script', 'PASS' if deletes and scripts and max(deletes) < min(scripts) else 'FAIL', dict(removalAttemptLogLines=deletes, installScriptLogLines=scripts, note='Closed baseline only; this does not establish locked-file order/outcome.'))
for label in ['closed-B-direct-and-public-shim', 'running-A-PID-survival-before-release', 'stopped-locked-executable', 'stopped-locked-resource', 'ordinary-closed-B-retry-and-fresh-launches', 'final-synthetic-data-preservation', 'cooperative-teardown-and-cleanup', 'resolved-shim-target']:
    check(label, 'NOT_ESTABLISHED', 'No completed captured assertion; raw shim embedded-target search was false, not proof of a broken shim.')
check('complete-transcript', 'FAIL', 'Guest transcript unavailable: collection raised sharing/read error. Probe process exit/output absent from host records. Preserved command stdout is not a full probe transcript.')
result = dict(run=build['run'], clone='{SANITIZED_VM_2}', overall='BLOCKED_INCOMPLETE_NOT_ACCEPTANCE_PASS', safetyBoundary='Retained read-only final inspection: prlctl exit 255, Unable to open new session. Earlier runner status unresolved. No rerun, kill, concurrent manager mutation, restoration, or cleanup attempted by offline audit.', checks=checks, capturedCommands=commands, metadata=snapshot['metadata'], shim=dict(path=snapshot['shim'], expectedTarget=snapshot['expectedTarget'], embeddedTargetSearch=snapshot['shimContainsExpectedTarget']), optionalBackstop='Not attempted; running baseline did not complete.', remainingMatrix=['native amd64/production PE and resource closure', 'cross-user/standard-user/alternate-admin and real user-data boundaries', 'relaunch/races/inaccessible-process inspection', 'agent/worker/reconnect coordination', 'standalone/convenience/partial upgrades and uninstall locks', 'installed paths with spaces/Unicode', 'interruption/power loss/permission/disk failures', 'signing/runtime delivery'], evidenceIntegrity={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in [out/'native-go-build.json', out/'native-go-evidence.json', out/'native-go-host-transcript.json', out/'native-go-final-inspection.json']})
(out/'native-go-result.json').write_text(json.dumps(result, indent=2)+'\n')
print(json.dumps(dict(overall=result['overall'], checks=[dict(label=x['label'],status=x['status']) for x in checks]), indent=2))
