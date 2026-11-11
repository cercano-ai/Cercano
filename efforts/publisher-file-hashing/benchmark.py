import importlib.util
import json
import os
from pathlib import Path
import resource
import shutil
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
if len(sys.argv) == 1:
    results = []
    for mode in ('baseline', 'candidate', 'candidate', 'baseline'):
        result = json.loads(subprocess.check_output([sys.executable, __file__, mode], text=True))
        results.append(result)
        print(json.dumps(result), flush=True)
    assert all(row['identity'] == results[0]['identity'] for row in results)
    assert all(row['tree_digests'] == results[0]['tree_digests'] for row in results)
    (HERE / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
else:
    mode = sys.argv[1]
    script = HERE / 'baseline.py' if mode == 'baseline' else ROOT / 'source/server/scripts/publish-dev-generation.py'
    spec = importlib.util.spec_from_file_location('publisher', script)
    p = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(p)
    env, _ = p.build_environment()
    env['GOPROXY'] = 'off'
    start = time.perf_counter()
    identity = p.effective_identity(shutil.which('go'), env, ROOT / 'source/server')
    elapsed = time.perf_counter() - start
    import hashlib
    print(json.dumps({'mode': mode, 'wall_seconds': elapsed,
                      'maxrss_bytes_darwin': resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,
                      'identity': hashlib.sha256(json.dumps(identity, sort_keys=True).encode()).hexdigest(),
                      'tree_digests': {k: v for k, v in identity.items() if k != 'go_env'}}))
