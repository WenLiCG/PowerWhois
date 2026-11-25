import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import crypto from 'node:crypto';

const source = fs.readFileSync(new URL('../src/components/BulkCheck.vue', import.meta.url), 'utf8');
const code = source.slice(source.indexOf('function showError('), source.indexOf('async function control('));
function fixture() {
  const ctx = {
    error: {value: ''}, refreshError: {value: ''}, refreshing: {value: false}, creating: {value: false},
    selectedID: {value: 'prior-task'}, task: {value: {id: 'prior-task'}}, tasks: {value: []},
    results: {value: []}, resultOffset: {value: 0}, resultOffsets: {value: [0]}, nextResultOffset: {value: -1},
    historyOffset: {value: 0}, resultRequest: 0, refreshRequest: 0, loadingResults: {value: false}, pageStatusFilter: {value: 'all'}, stopped: false, mode: {value: 'dnsQuery'},
    concurrency: {value: 15}, timeout: {value: 30}, retries: {value: 0},
    file: {value: {size: 32, text: async () => 'example.com\nbad name.com'}},
    submissionBody: null, submissionKey: null, crypto,
    localStorage: {setItem() {}, removeItem() {}},
    api: {
      get: async path => ({data: path.endsWith('/results') ? {items: [], nextOffset: -1} : path === '/v1/tasks' ? {items: [{id: 'prior-task'}]} : {id: 'prior-task'}}),
      post: async () => {throw {response: {data: {error: {message: 'invalid domain at line 2'}}}};}
    }
  };
  vm.createContext(ctx); vm.runInContext(code, ctx); return ctx;
}

test('background polls preserve a rejected upload message, file and previous task', async () => {
  const ctx = fixture(); await ctx.createTask();
  const file = ctx.file.value, key = ctx.submissionKey;
  for (let poll = 0; poll < 3; poll++) await ctx.refresh();
  assert.equal(ctx.error.value, 'invalid domain at line 2');
  assert.equal(ctx.file.value, file); assert.equal(ctx.selectedID.value, 'prior-task');
  assert.equal(ctx.submissionKey, key);
});

test('refresh failures recover without hiding the separate upload error', async () => {
  const ctx = fixture(); await ctx.createTask(); const get = ctx.api.get;
  ctx.api.get = async () => {throw {response: {data: {error: {message: 'refresh unavailable'}}}};};
  await ctx.refresh();
  assert.equal(ctx.error.value, 'invalid domain at line 2');
  assert.equal(ctx.refreshError.value, 'refresh unavailable');
  ctx.api.get = get; await ctx.refresh();
  assert.equal(ctx.refreshError.value, '');
  assert.equal(ctx.error.value, 'invalid domain at line 2');
  ctx.api.post = async () => ({data: {id: 'created-task'}});
  await ctx.createTask();
  assert.equal(ctx.error.value, ''); assert.equal(ctx.file.value, null);
  assert.equal(ctx.selectedID.value, 'created-task');
});

test('a late missing task response cannot clear a newer selection', async () => {
  const ctx = fixture();
  let rejectOld, oldRequested;
  const requested = new Promise(resolve => {oldRequested = resolve;});
  const get = ctx.api.get;
  ctx.api.get = async path => {
    if (path === '/v1/tasks/prior-task') {
      oldRequested();
      return new Promise((_, reject) => {rejectOld = reject;});
    }
    if (path === '/v1/tasks/new-task') return {data: {id: 'new-task'}};
    return get(path);
  };
  const oldRefresh = ctx.refresh();
  await requested;
  ctx.selectedID.value = 'new-task';
  await ctx.loadSelected();
  rejectOld({response: {status: 404}});
  await oldRefresh;
  assert.equal(ctx.task.value.id, 'new-task');
  assert.equal(ctx.selectedID.value, 'new-task');
  assert.equal(ctx.refreshError.value, '');
});
