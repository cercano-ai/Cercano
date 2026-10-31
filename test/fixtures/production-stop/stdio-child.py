"""Disposable external stdio MCP fixture; no credentials/network/config reads."""
import json, os, pathlib, signal, subprocess, sys
root = pathlib.Path(sys.argv[1])
signal.alarm(90)
child = subprocess.Popen(['/bin/sleep', '90'])
(root / ('stdio-' + str(os.getppid()) + '.json')).write_text(json.dumps({'pid': os.getpid(), 'child': child.pid}))
try:
    for line in sys.stdin:
        request = json.loads(line)
        if 'id' not in request:
            continue
        method = request.get('method')
        result = {}
        if method == 'initialize':
            result = {'protocolVersion': request['params']['protocolVersion'], 'capabilities': {'tools': {}}, 'serverInfo': {'name': 'disposable', 'version': '1'}}
        elif method == 'tools/list':
            result = {'tools': []}
        print(json.dumps({'jsonrpc': '2.0', 'id': request['id'], 'result': result}), flush=True)
finally:
    child.terminate()
    child.wait()
