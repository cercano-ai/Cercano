#!/usr/bin/env python3
"""Synthetic stdio MCP peer; only launched by the private compiled rehearsal."""
import json
import os
import pathlib
import sys

assert os.environ.get('CERCANO_DISABLE_CREDENTIALS') == '1'
log = pathlib.Path(__file__).with_name('mcp.jsonl')
for line in sys.stdin:
    request = json.loads(line)
    method = request.get('method')
    if 'id' not in request:
        continue
    if method == 'initialize':
        result = {'protocolVersion': request['params']['protocolVersion'],
                  'capabilities': {'tools': {}},
                  'serverInfo': {'name': 'offline-fixture', 'version': '1'}}
    elif method == 'tools/list':
        result = {'tools': [{'name': 'echo', 'description': 'Offline synthetic echo',
                            'inputSchema': {'type': 'object', 'properties': {'text': {'type': 'string'}}, 'required': ['text']}}]}
    elif method == 'tools/call':
        assert request['params']['name'] == 'echo'
        text = request['params']['arguments']['text']
        assert text == 'FIXTURE_MCP_RESULT'
        with log.open('a') as output:
            output.write(json.dumps({'event': 'tools/call', 'pid': os.getpid(), 'text': text}) + '\n')
        result = {'content': [{'type': 'text', 'text': text}], 'isError': False}
    elif method == 'ping':
        result = {}
    else:
        print(json.dumps({'jsonrpc': '2.0', 'id': request['id'], 'error': {'code': -32601, 'message': 'unsupported fixture method'}}), flush=True)
        continue
    print(json.dumps({'jsonrpc': '2.0', 'id': request['id'], 'result': result}), flush=True)
