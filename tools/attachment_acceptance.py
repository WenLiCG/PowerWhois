#!/usr/bin/env python3
"""VPS attachment regression. No registration; preserves prior tasks and writes a JSON report.
Run: python3 attachment_acceptance.py FIXTURE_DIR
Requires web-cases.json, typo-comparison.json, original.txt and txt-valid-unique.txt.
Web means its real HTTP/WebSocket contracts, not a browser UI session.
"""
import base64,collections,csv,hashlib,io,json,os,socket,struct,subprocess,sys,time,urllib.request,urllib.error
from pathlib import Path

ROOT=Path(sys.argv[1]);REPORT=ROOT/'attachment-live.json'
ENV=dict(line.split('=',1) for line in Path('/etc/powerwhois/service.env').read_text().splitlines() if '=' in line and not line.startswith('#'))
BASE='http://127.0.0.1:8080';TOKEN=ENV['POWERWHOIS_QUERY_TOKEN']
report={'checks':[],'tasks':[],'websocket':[],'startedAt':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime())}
def save():
 REPORT.write_text(json.dumps(report,ensure_ascii=False,indent=2));os.chmod(REPORT,0o600)
def check(name,condition):
 if not condition:raise AssertionError(name)
 report['checks'].append(name);save();print('PASS:',name,flush=True)
def request(path,body=None,token=TOKEN,key=None,raw=False):
 headers={'Content-Type':'application/json','Authorization':'Bearer '+token}
 if key:headers['Idempotency-Key']=key
 req=urllib.request.Request(BASE+path,data=None if body is None else json.dumps(body).encode(),headers=headers)
 with urllib.request.urlopen(req,timeout=130) as r:
  data=r.read();return (data,dict(r.headers)) if raw else json.loads(data)
def cli(*args,valid=True):
 r=subprocess.run(['/opt/powerwhois/current/powerwhois',*args],env={**os.environ,**ENV,'POWERWHOIS_TOKEN_FILE':''},capture_output=True,timeout=130)
 if not valid:return r
 assert r.returncode==0,(r.returncode,r.stderr.decode()[:200])
 return json.loads(r.stdout)
def mcp(name,args):
 req=urllib.request.Request('http://127.0.0.1:8081/mcp',data=json.dumps({'jsonrpc':'2.0','id':1,'method':'tools/call','params':{'name':name,'arguments':args}}).encode(),headers={'Content-Type':'application/json','Accept':'application/json, text/event-stream','Authorization':'Bearer '+ENV['POWERWHOIS_MCP_TOKEN']})
 with urllib.request.urlopen(req,timeout=130) as r:result=json.load(r)
 assert 'error' not in result,result
 result=result['result'];assert not result.get('isError'),result
 return result['structuredContent']
def all_results(task):
 items=[];offset=0
 while offset>=0:
  page=request(f"/api/v1/tasks/{task}/results?offset={offset}&limit=1000")
  items.extend(page['items']);next_offset=page['nextOffset'];assert next_offset==-1 or next_offset>offset;offset=next_offset
 return items
def wait_task(task,seconds=900):
 deadline=time.monotonic()+seconds;last=0
 while time.monotonic()<deadline:
  current=request('/api/v1/tasks/'+task)
  if time.monotonic()-last>25:
   print('progress:',task,current['completed'],current['total'],current['state'],flush=True);last=time.monotonic()
  if current['state']=='completed':return current
  assert current['state'] not in ('failed','canceled'),current
  time.sleep(1)
 raise AssertionError('task deadline exceeded')
class WS:
 def __init__(self):
  self.sock=socket.create_connection(('127.0.0.1',8080),timeout=90)
  key=base64.b64encode(os.urandom(16)).decode()
  self.sock.sendall(('GET /app/ws HTTP/1.1\r\nHost: 127.0.0.1:8080\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: '+key+'\r\nOrigin: http://127.0.0.1:8080\r\n\r\n').encode())
  headers=b''
  while not headers.endswith(b'\r\n\r\n'):headers+=self.read(1)
  assert b'101 Switching Protocols' in headers,headers
 def read(self,n):
  data=b''
  while len(data)<n:
   chunk=self.sock.recv(n-len(data));assert chunk,'WebSocket disconnected';data+=chunk
  return data
 def send(self,event,data):
  payload=json.dumps({'event':event,'data':data}).encode();mask=os.urandom(4);n=len(payload)
  header=bytes([0x81,0x80|n]) if n<126 else bytes([0x81,0xfe])+struct.pack('!H',n) if n<65536 else bytes([0x81,0xff])+struct.pack('!Q',n)
  self.sock.sendall(header+mask+bytes(b^mask[i%4] for i,b in enumerate(payload)))
 def receive(self):
  fragments=b''
  while True:
   first,second=self.read(2);opcode=first&15
   n=second&127
   if n==126:n=struct.unpack('!H',self.read(2))[0]
   if n==127:n=struct.unpack('!Q',self.read(8))[0]
   assert n<=4*1024*1024 and not second&128
   payload=self.read(n)
   if opcode==10:continue
   if opcode==9:
    assert n<=125
    mask=os.urandom(4)
    self.sock.sendall(bytes([0x8a,0x80|n])+mask+bytes(b^mask[i%4] for i,b in enumerate(payload)))
    continue
   assert opcode in (0,1),(first,second,payload[:100])
   fragments+=payload;assert len(fragments)<=4*1024*1024
   if first&128:return json.loads(fragments)
 def close(self):self.sock.close()

try:
 cases=json.loads((ROOT/'web-cases.json').read_text());typos=json.loads((ROOT/'typo-comparison.json').read_text())
 prior=request('/api/v1/tasks?limit=100')['items'];report['priorTaskIds']=[t['id'] for t in prior]
 assert not any(t['state'] in ['queued','running'] for t in prior),'existing task active; defer live suite'
 admin=request('/api/login',{'username':ENV['POWERWHOIS_ADMIN_USER'],'password':ENV['POWERWHOIS_ADMIN_PASSWORD']})['token']
 config=request('/api/admin/setting',token=admin)
 check('live DNS/WHOIS/Web defaults are 15',all(config[k]==15 for k in ['dnsConcurrency','whoisConcurrency','webCheckConcurrencyLimit']))
 report['protocolLimits']={k:config[k] for k in ['dnsConcurrency','whoisConcurrency','webCheckConcurrencyLimit','rdapConcurrency','dnsRate','whoisRate','rdapRate']}
 coverage=request('/api/v1/query',{'domains':['google.cf','ox.ac.uk'],'mode':'whois'})['items']
 check('unsupported and delegated suffixes retain truthful local evidence',all(r['status']=='unknown' and r['source']=='local-validation' and r['queryError'] for r in coverage))
 # Invalid originals are rejected atomically through both machine and authenticated Web APIs.
 original=[s.strip() for s in (ROOT/'original.txt').read_text(encoding='utf-8-sig').splitlines() if s.strip()]
 for label,credential in [('AI API',TOKEN),('Web API',admin)]:
  try:request('/api/v1/tasks',{'domains':original,'mode':'dns'},token=credential);raise AssertionError('invalid original accepted')
  except urllib.error.HTTPError as e:
   payload=json.load(e);check(label+' rejects original TXT with input position',e.code==400 and '1284' in payload['error']['message'])
 bad=cli('tasks','create','--file',str(ROOT/'original.txt'),'--mode','dns',valid=False)
 check('CLI rejects invalid TXT on stderr with exit 2',bad.returncode==2 and not bad.stdout and b'1284' in bad.stderr)
 check('invalid imports leave no task',len(request('/api/v1/tasks?limit=100')['items'])==len(prior))
 # Submit all normalized entries once, pause immediately, replay via other entrances, then cancel.
 valid=(ROOT/'txt-valid-unique.txt').read_text().splitlines();key='attachments-full-'+hashlib.sha256('\n'.join(valid).encode()).hexdigest()[:20]+'-'+str(int(time.time()))
 body={'domains':valid,'mode':'dns','concurrency':15,'timeout':30,'retries':0}
 started=time.monotonic();large=request('/api/v1/tasks',body,token=admin,key=key);large_id=large['id']
 request('/api/v1/tasks/'+large_id+'/pause',{})
 check('full Web API import count and default',large['total']==len(valid)==554867 and large['concurrency']==15)
 replay=cli('tasks','create','--file',str(ROOT/'txt-valid-unique.txt'),'--mode','dns','--key',key)
 check('full CLI import reuses the same task',replay['id']==large_id)
 # Read/control/export through MCP without putting a half-million names in a model response.
 snapshot=mcp('get_task',{'id':large_id});check('MCP reads full imported task',snapshot['total']==554867 and snapshot['state']=='paused')
 snapshot=request('/api/v1/tasks/'+large_id+'/cancel',{})
 partial=request('/api/v1/tasks/'+large_id+'/export?limit=1000')
 check('partial export identical across Web/API/CLI/MCP',partial==request('/api/v1/tasks/'+large_id+'/export?limit=1000',token=admin)==cli('tasks','export','--id',large_id,'--limit','1000')==mcp('export_task',{'id':large_id,'limit':1000}))
 _,headers=request('/api/v1/tasks/'+large_id+'/export?format=csv',raw=True)
 check('canceled export explicitly marked partial',headers.get('X-Export-Consistency')=='partial')
 report['fullImport']={'task':snapshot,'seconds':time.monotonic()-started,'liveQueriesBoundedByImmediatePause':True};save()
 # Invalid spreadsheet cases: same strict rejection in WebSocket and machine API; no partial queries.
 for case in cases:
  if not case['rejected']:continue
  try:request('/api/v1/query',{'domains':case['raw'][:100],'mode':'dns'});raise AssertionError('invalid case accepted')
  except urllib.error.HTTPError as e:check('API invalid case '+case['name'],e.code==400)
  ws=WS();ws.send('webCheck',{'domains':case['raw'],'queryType':'dnsQuery'});response=ws.receive();ws.close()
  check('WebSocket invalid case '+case['name'],response['event']=='webCheckError' and '第 ' in response['data'] and '\ufffd' not in response['data'])
 # Typo generation uses no upstream requests. All 12 bases use the worksheet's seven generators.
 kinds=['www','skipLetter','doubleLetter','reverseLetter','insertedLetter','wrongHorizontalKey','wrongVerticalKey']
 for domain,expected in typos['generated'].items():
  args={'domain':domain,'types':kinds}
  actual=request('/api/v1/typo',args)
  check('Typo API/CLI/MCP '+domain,actual==expected==cli('typo','--domain',domain,'--types',','.join(kinds))==mcp('generate_typos',args))
 # Execute every taken/free entry, and all valid edge-case entries, for DNS and WHOIS.
 domains=list(dict.fromkeys(d for case in cases for d in case['validUnique']))
 report['realCorpusCount']=len(domains)
 for mode in ['dns','whois']:
  body={'domains':domains,'mode':mode,'timeout':30,'retries':0}
  task=mcp('create_task',{**body,'idempotencyKey':'attachments-live-'+mode+'-'+str(int(time.time()))})
  check(mode+' MCP default concurrency is 15',task['concurrency']==15)
  task=wait_task(task['id']);rows=all_results(task['id'])
  check(mode+' all real input entries accounted',len(rows)==len(domains) and [r['domain'] for r in rows]==domains and task['completed']==len(domains) and sum(task['counts'].values())==len(domains))
  check(mode+' evidence has source and time',all(r['source'] and r['queriedAt'] for r in rows))
  export=request('/api/v1/tasks/'+task['id']+'/export?limit=1000')
  check(mode+' complete exports identical across four channels',export==request('/api/v1/tasks/'+task['id']+'/export?limit=1000',token=admin)==cli('tasks','export','--id',task['id'],'--limit','1000')==mcp('export_task',{'id':task['id'],'limit':1000}))
  csv_bytes,h=request('/api/v1/tasks/'+task['id']+'/export?format=csv&limit=1000',raw=True)
  check(mode+' CSV complete and all rows retained',h.get('X-Export-Consistency')=='complete' and len(list(csv.DictReader(io.StringIO(csv_bytes.decode()))))==len(domains))
  report['tasks'].append({'task':task,'results':rows});save()
  ws=WS();ws.send('webCheck',{'domains':domains,'queryType':mode+'Query'})
  message=ws.receive();check(mode+' WebSocket normalized list',message['event']=='webCheckDomains' and message['data']==domains)
  ws_rows={};started=time.monotonic()
  while len(ws_rows)<len(domains):
   message=ws.receive();assert message['event']=='webCheckResult',message
   row=message['data'];assert row['domain'] not in ws_rows;ws_rows[row['domain']]=row
   if len(ws_rows)%50==0:print('WebSocket',mode,len(ws_rows),'/',len(domains),flush=True)
   assert time.monotonic()-started<900
  ws.close()
  differences=[{'domain':r['domain'],'task':r['status'],'websocket':ws_rows[r['domain']]['status'],'taskSource':r['source'],'webSource':ws_rows[r['domain']]['source']} for r in rows if r['status']!=ws_rows[r['domain']]['status']]
  # Live registries can time out or rate-limit; record differences for targeted investigation.
  report['websocket'].append({'mode':mode,'count':len(ws_rows),'counts':dict(collections.Counter(r['status'] for r in ws_rows.values())),'differences':differences,'results':list(ws_rows.values())});save()
  check(mode+' WebSocket returns every entry once',len(ws_rows)==len(domains))
 check('all prior task states and counts retained',all(request('/api/v1/tasks/'+t['id'])==t for t in prior))
 report['result']='PASS';save();print('PASS: attachment suite completed',flush=True)
except BaseException as e:
 report['result']='FAIL';report['failure']=str(e);save();raise
