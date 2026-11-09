#!/usr/bin/env python3
"""Assert captured metadata, changed/preserved hashes, shim state and cleanup.

Not checked: expected B bytes/resources, public shim launch, PID survival,
retry outcomes, or complete resource cleanup. No guest execution is performed.
"""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3] / 'efforts/cross-platform-updates'
EXPECTED = {
    'phase1': {'closed': (0, '2.0.0'), 'beforethrow': (-1, '2.0.0'),
               'beforenonzero': (19, '2.0.0'), 'running': (0, '2.0.0'),
               'locked': (1, '1.0.0'), 'uninstallthrow': (-1, '1.0.0'),
               'installfail': (-1, '1.0.0')},
    'extensions': {'lockedthrow': (-1, '1.0.0'), 'shimlocked': (1, '2.0.0')},
    'smoke': {'closed': (0, '2.0.0')},
}
# The final parameterized run must be checked too, not merely the historical runs.
EXPECTED['consolidated'] = {**EXPECTED['phase1'], **EXPECTED['extensions']}
for group, cases in EXPECTED.items():
    d = json.loads((ROOT / f'windows-{group}-evidence.json').read_text(encoding='utf-8-sig'))
    commands = {r['label']: r for r in json.loads(d['commands.json'])}
    assert d['data-before.csv'] == d['data-after.csv'], group
    for case, (code, version) in cases.items():
        ident = 'cercano-phase1-' + case
        assert commands[case + '-operation']['exit'] == code
        assert f'{ident}|{version}' in d[case + '-after-metadata.txt']
        def snapshot(stage):
            return {r['path']: r['sha256'] for r in json.loads(d[f'{case}-{stage}-files.json'])}
        before, after, cleaned = snapshot('before'), snapshot('after'), snapshot('cleaned')
        payload = f'C:\\ProgramData\\chocolatey\\lib\\{ident}\\tools\\{ident}.exe'
        shim = f'C:\\ProgramData\\chocolatey\\bin\\{ident}.exe'
        if version == '2.0.0':
            assert after[payload] != before[payload], case
        else:
            assert all(after.get(p) == h for p, h in before.items() if p != shim), case
        if case == 'uninstallthrow':
            assert shim not in after
            assert 'uninstall:shim=False' in d[case + '\\hooks.txt']
        elif case == 'shimlocked':
            assert after[shim] == before[shim]
        else:
            assert shim in after
        assert commands[case + '-cleanup']['exit'] == 0
        assert payload not in cleaned and shim not in cleaned
        assert ident + '|' not in d[case + '-cleaned-metadata.txt']
        print(f'{group}/{case}: exit={code}, metadata={version}, executable changed/non-shim hashes preserved, shim state + cleanup asserted')
print('PASS: captured observations and synthetic data preservation (not full native acceptance)')
