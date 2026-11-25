#!/usr/bin/env python3
"""Run on a deployed VPS as its operator. Uses protected environment files, never prints secrets.
Only public example-domain lookups and query tasks are submitted. Registration is never executed.
This restarts powerwhois.service once; run during an authorized maintenance window.
"""
import base64
import io
import json
import os
from pathlib import Path
import re
import secrets
import socket
import struct
import subprocess
import time
import urllib.error
import urllib.request
import zipfile


def main():
    env = dict(line.split('=', 1) for line in Path(os.environ.get('POWERWHOIS_ENV_FILE', '/etc/powerwhois/service.env')).read_text().splitlines() if '=' in line and not line.startswith('#'))
    base = env.get('POWERWHOIS_URL', 'http://127.0.0.1:8080')
    token = env['POWERWHOIS_QUERY_TOKEN']
    report = {'checks': [], 'liveQueries': [], 'taskIds': []}

    def check(name, condition):
        assert condition, name
        report['checks'].append(name)
        print('PASS:', name, flush=True)

    def request(path, body=None, credential=token, method=None, raw=False, extra=None):
        req = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(),
            headers={'Content-Type': 'application/json', **({'Authorization': 'Bearer '+credential} if credential else {}), **(extra or {})}, method=method)
        with urllib.request.urlopen(req, timeout=130) as response:
            data = response.read()
            return (data, dict(response.headers)) if raw else json.loads(data)

    def rejected(path, status, body=None, credential='', method=None):
        try:
            request(path, body, credential, method)
        except urllib.error.HTTPError as error:
            return error.code == status
        return False

    html, _ = request('/', credential='', raw=True)
    check('native Web and SPA', b'PowerWhois' in html and request('/operator/check', credential='', raw=True)[0] == html)
    assets = re.findall(rb'(?:src|href)="([^"]+\.(?:js|css))"', html)
    check('production frontend assets', bool(assets) and all(request(p.decode(), credential='', raw=True)[0] for p in assets))
    check('unknown API is not SPA HTML', rejected('/api/unknown', 404))
    check('anonymous API rejected', rejected('/api/v1/tasks', 401))
    check('query token cannot administer', rejected('/api/admin/setting', 401, credential=token))
    admin = request('/api/login', {'username': env['POWERWHOIS_ADMIN_USER'], 'password': env['POWERWHOIS_ADMIN_PASSWORD']}, credential='')['token']
    settings = request('/api/admin/setting', credential=admin)
    check('settings hide credentials', not any(settings[k] for k in ('authPassword', 'jwtSecretKey', 'socketProxyPassword')))
    settings['secretActions'] = {'authPassword': 'keep', 'jwtSecretKey': 'keep', 'socketProxyPassword': 'keep'}
    saved = request('/api/admin/setting', settings, admin, 'PUT')
    check('native settings atomic save', saved['dnsRate'] == settings['dnsRate'] and saved['authPassword'] == '')
    check('invalid domain rejected', rejected('/api/v1/tasks', 400, {'domains': ['a b.com']}, token))
    kinds=['www','skipLetter','doubleLetter','reverseLetter','insertedLetter','wrongHorizontalKey','wrongVerticalKey','customizedReplace']
    generated=request('/api/v1/typo',{'domain':'example.com','types':kinds,'tlds':['com','net','org']})
    check('all Typo generators and suffix generation',set(generated['groups'])==set(kinds+['wrongTlds']) and bool(generated['domains']) and len(generated['domains'])==len(set(generated['domains'])))

    unique = 'powerwhois-vps-' + secrets.token_hex(8) + '.com'
    rows = request('/api/v1/query', {'domains': ['example.com', 'example.org', unique], 'mode': 'whois', 'timeout': 30, 'concurrency': 2})['items']
    report['liveQueries'] = [{k: r[k] for k in ('domain', 'status', 'source', 'queriedAt', 'queryError')} for r in rows]
    check('real RDAP registered/absent', [r['status'] for r in rows] == ['registered', 'registered', 'not_found'])
    absent=request('/api/v1/query',{'domains':[unique],'mode':'dns','timeout':20})['items'][0]
    check('real DNS absence stays unknown',absent['status']=='unknown')
    for mode, domain in (('dns', 'example.com'), ('whois', 'example.de'), ('mixed', 'example.org')):
        row = request('/api/v1/query', {'domains': [domain], 'mode': mode, 'timeout': 20})['items'][0]
        report['liveQueries'].append({**{k: row[k] for k in ('domain', 'status', 'source', 'queryError')}, 'mode': mode})
        check('real '+mode+' query', row['status'] == 'registered')

    run_env = {**os.environ, **env, 'POWERWHOIS_TOKEN_FILE': ''}
    binary = '/opt/powerwhois/current/powerwhois'
    def cli(*args):
        result = subprocess.run([binary, *args], env=run_env, capture_output=True, timeout=130)
        assert result.returncode == 0, 'CLI failed without exposing diagnostic payload'
        return json.loads(result.stdout)
    task = cli('tasks', 'create', '--key', 'vps-smoke-'+secrets.token_hex(5), '--mode', 'whois', 'example.com', 'example.org')
    report['taskIds'].append(task['id'])
    for _ in range(100):
        if request('/api/v1/tasks/'+task['id'])['state'] == 'completed': break
        time.sleep(.2)
    else: raise AssertionError('live task completion timeout')
    snapshot = request('/api/v1/tasks/'+task['id']+'/export')
    check('CLI/API task and export agreement', snapshot == cli('tasks', 'export', '--id', task['id']))
    evidence = request('/api/v1/tasks/'+task['id']+'/results/0')
    check('on-demand complete evidence', bool(evidence['rawResponse']))
    csv, headers = request('/api/v1/tasks/'+task['id']+'/export?format=csv', raw=True)
    check('CSV completeness metadata', headers.get('X-Export-Consistency') == 'complete' and b'example.com' in csv)

    mcp_token = env['POWERWHOIS_MCP_TOKEN']
    def mcp(method, params, authorized=True):
        req = urllib.request.Request('http://127.0.0.1:8081/mcp', data=json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params}).encode(),
            headers={'Content-Type':'application/json','Accept':'application/json, text/event-stream', **({'Authorization':'Bearer '+mcp_token} if authorized else {})})
        with urllib.request.urlopen(req, timeout=130) as response: return json.load(response)
    try:
        mcp('initialize', {}, False)
        raise AssertionError('MCP anonymous request accepted')
    except urllib.error.HTTPError as error: check('MCP bearer enforced', error.code == 401)
    check('MCP HTTP initialize', 'result' in mcp('initialize', {'protocolVersion':'2025-06-18','capabilities':{},'clientInfo':{'name':'vps-acceptance','version':'1'}}))
    tools = mcp('tools/list', {})['result']['tools']
    check('MCP query-only tool inventory', len(tools) == 11 and not any('register' in t['name'] or 'delete' in t['name'] for t in tools))
    exported = mcp('tools/call', {'name':'export_task','arguments':{'id':task['id']}})['result']['structuredContent']
    check('MCP/API export agreement', exported == snapshot)

    def ws(origin='http://127.0.0.1:8080'):
        sock = socket.create_connection(('127.0.0.1',8080), timeout=30)
        key = base64.b64encode(os.urandom(16)).decode()
        sock.sendall(('GET /app/ws HTTP/1.1\r\nHost: 127.0.0.1:8080\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: '+key+'\r\nOrigin: '+origin+'\r\n\r\n').encode())
        header=b''
        while not header.endswith(b'\r\n\r\n'): header+=sock.recv(1)
        return sock,header
    def send(sock,event,data):
        data=json.dumps({'event':event,'data':data}).encode(); mask=os.urandom(4)
        header=bytes([0x81,0x80|len(data)]) if len(data)<126 else bytes([0x81,0xfe])+struct.pack('!H',len(data))
        sock.sendall(header+mask+bytes(b^mask[i%4] for i,b in enumerate(data)))
    def receive(sock):
        def read(n):
            data=b''
            while len(data)<n:
                chunk=sock.recv(n-len(data));assert chunk,'WebSocket disconnected';data+=chunk
            return data
        first,second=read(2);length=second&127
        if length==126:length=struct.unpack('!H',read(2))[0]
        if length==127:length=struct.unpack('!Q',read(8))[0]
        return json.loads(read(length))
    sock,header=ws('https://untrusted.example');sock.close();check('WebSocket Origin rejected', b'403' in header)
    sock,header=ws();check('WebSocket upgrade',b'101' in header)
    send(sock,'register',{'domains':['example.com'],'registerType':'not-configured','confirmed':True})
    check('anonymous WebSocket cannot register',receive(sock)['event']=='registerError')
    send(sock,'adminAuth',admin)
    send(sock,'register',{'domains':['example.com'],'registerType':'not-configured','confirmed':False})
    check('registration requires explicit confirmation',receive(sock)['event']=='registerError')
    send(sock,'webCheck',{'domains':['example.com'],'queryType':'whoisQuery'})
    result=None
    for _ in range(10):
        message=receive(sock)
        if message['event']=='webCheckResult':result=message['data'];break
    sock.close();check('real WebSocket query',result is not None and result['status']=='registered')

    data,_=request('/api/admin/log',credential=admin,raw=True)
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        text='\n'.join(archive.read(name).decode(errors='replace') for name in archive.namelist())
        check('protected log download and secret scan', archive.testzip() is None and not any(env[k] in text for k in ('POWERWHOIS_ADMIN_PASSWORD','POWERWHOIS_JWT_SECRET','POWERWHOIS_QUERY_TOKEN','REDIS_PASSWORD')))
    subprocess.run(['systemctl','restart','powerwhois.service'],check=True)
    for _ in range(200):
        try:
            restored=request('/api/v1/tasks/'+task['id']+'/export');break
        except (urllib.error.URLError,ConnectionError):time.sleep(.2)
    else:raise AssertionError('systemd restart readiness timeout')
    check('systemd restart preserves export',restored==snapshot)
    check('MCP remains usable after backend restart',mcp('tools/list',{})['result']['tools']==tools)
    report['result']='PASS'
    destination=Path('/opt/powerwhois/verification/vps-r2.json');destination.write_text(json.dumps(report,indent=2));os.chmod(destination,0o600)
    print('PASS: deployed service acceptance; report saved without credentials',flush=True)


if __name__=='__main__':
    main()
