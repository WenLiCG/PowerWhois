import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

test('late result responses cannot replace a newer page', async () => {
  const source=fs.readFileSync(new URL('../src/components/BulkCheck.vue',import.meta.url),'utf8');
  const code=source.slice(source.indexOf('async function loadResults()'),source.indexOf('async function loadSelected()'));
  const pending=[];
  const context={selectedID:{value:'task-a'},resultOffset:{value:0},results:{value:[]},nextResultOffset:{value:-1},resultRequest:0,loadingResults:{value:false},stopped:false,showError:e=>{throw e},api:{get:()=>new Promise((resolve,reject)=>pending.push({resolve,reject}))}};
  vm.createContext(context); vm.runInContext(code,context);
  const first=context.loadResults();context.resultOffset.value=100;const second=context.loadResults();
  pending[1].resolve({data:{items:['new-page'],nextOffset:200}});await second;
  pending[0].resolve({data:{items:['old-page'],nextOffset:100}});await first;
  assert.equal(context.results.value[0],'new-page');assert.equal(context.nextResultOffset.value,200);
  pending.length=0; const old=context.loadResults(); const current=context.loadResults();
  pending[1].resolve({data:{items:['current'],nextOffset:-1}});await current;
  pending[0].reject(new Error('obsolete network error'));await old;
  assert.equal(context.results.value[0],'current');
});

test('task cancellation stays bound to the task shown at confirmation', async () => {
  const source = fs.readFileSync(new URL('../src/components/BulkCheck.vue',import.meta.url),'utf8');
  const code = source.slice(source.indexOf('async function control('),source.indexOf('async function copyID()'));
  const requests = [];
  let confirm;
  const ctx = {
    selectedID: {value:'task-a'}, acting: {value:false}, stopped:false, task:{value:{}},
    $q: {dialog: () => ({onOk: fn => {confirm = fn;}})},
    api: {post: async url => {requests.push(url);return {data:{}};}}, refresh:async () => {}
  };
  vm.createContext(ctx); vm.runInContext(code,ctx);
  ctx.confirmCancel();
  ctx.selectedID.value = 'task-b';
  await confirm();
  assert.deepEqual(requests, []);
  ctx.confirmCancel();
  await confirm();
  assert.deepEqual(requests, ['/v1/tasks/task-b/cancel']);
});

test('registration sends and marks only the confirmed domains and provider', () => {
  const source = fs.readFileSync(new URL('../src/components/modules/CheckedResult.vue',import.meta.url),'utf8');
  const code = source.slice(source.indexOf('function registerSelectedDomains()'),source.indexOf('onMounted(()'));
  const first = {domain:'one.com'}, second = {domain:'two.com'}, sent = [], marked = [];
  let confirm;
  const ctx = {
    selectedDomains:{value:[first]}, props:{registerType:'fixture-a'}, tokenStore:{token:'admin-session'},
    settingStore:{registerApis:['fixture-a','fixture-b']},
    webStore:{busy:false,domains:[first,second],setRegisteringDomains: rows => marked.push(rows.map(r => r.domain).join(','))},
    isWebsocketConnected:() => true, send: message => {sent.push(JSON.parse(message));return true;},
    $q:{dialog:() => ({onOk:fn => {confirm = fn;}}),notify:() => {}}
  };
  vm.createContext(ctx); vm.runInContext(code,ctx);
  ctx.registerSelectedDomains();
  ctx.selectedDomains.value = [second];
  ctx.props.registerType = 'fixture-b';
  confirm();
  assert.deepEqual(sent[0].data, {registerType:'fixture-a',confirmed:true,domains:['one.com']});
  assert.deepEqual(marked, ['one.com']);
  ctx.registerSelectedDomains();
  ctx.tokenStore.token = null;
  confirm();
  assert.equal(sent.length, 1);
});
test('batch detail errors stay in the open dialog and can be retried', async () => {
  const source = fs.readFileSync(new URL('../src/components/BulkCheck.vue', import.meta.url), 'utf8');
  const code = source.slice(source.indexOf('async function openDetails('), source.indexOf('function onVisibilityChange('));
  const pending = [];
  const context = {
    selectedID: {value: 'batch'}, details: {value: null}, detailError: {value: ''},
    loadingDetail: {value: false}, stopped: false, detailRequest: 0,
    api: {get: () => new Promise((resolve, reject) => pending.push({resolve, reject}))}
  };
  vm.createContext(context); vm.runInContext(code, context);
  const old = context.openDetails({order: 0}), current = context.openDetails({order: 1});
  pending[0].reject(new Error('stale')); await old;
  assert.equal(context.loadingDetail.value, true);
  assert.equal(context.detailError.value, '');
  pending[1].reject(new Error('offline')); await current;
  assert.equal(context.loadingDetail.value, false);
  assert.match(context.detailError.value, /读取原始响应失败/);
  const retry = context.openDetails(context.details.value);
  pending[2].resolve({data: {order: 1, rawResponse: 'body'}}); await retry;
  assert.equal(context.details.value.rawResponse, 'body');
  assert.equal(context.detailError.value, '');
});

test('reusing the saved proxy password on a changed host requires an explicit choice', async () => {
  const source = fs.readFileSync(new URL('../src/components/AdminSetting.vue', import.meta.url), 'utf8');
  const code = source.slice(source.indexOf('function confirmProxyPasswordReuse('), source.indexOf('function validateProxyHost('));
  const context = {
    savedProxyHost: 'old.test', settings: {value: {secrets: {socketProxyPassword: {configured: true}}}},
    $q: {dialog(options) { context.dialog = options; return {onOk(fn) {context.ok = fn; return this;}, onDismiss(fn) {context.dismiss = fn;}}; }}
  };
  vm.createContext(context); vm.runInContext(code, context);
  const payload = {socketProxyHost: 'OLD.TEST', socketProxyPort: 1080, socketProxyAuth: true, secretActions: {socketProxyPassword: 'keep'}};
  assert.equal(context.confirmProxyPasswordReuse(payload, true), true);
  payload.socketProxyHost = 'new.test';
  const cancel = context.confirmProxyPasswordReuse(payload, true);
  assert.match(context.dialog.message, /new\.test:1080/);
  context.dismiss(); assert.equal(await cancel, false);
  payload.socketProxyAuth = false;
  assert.equal(context.confirmProxyPasswordReuse(payload, true), true);
  const save = context.confirmProxyPasswordReuse(payload, false);
  context.ok(); context.dismiss(); assert.equal(await save, true);
  payload.secretActions.socketProxyPassword = 'set';
  assert.equal(context.confirmProxyPasswordReuse(payload, false), true);
});
