import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

test('late result responses cannot replace a newer page', async () => {
  const source=fs.readFileSync(new URL('../src/components/BulkCheck.vue',import.meta.url),'utf8');
  const code=source.slice(source.indexOf('async function loadResults()'),source.indexOf('async function loadSelected()'));
  const pending=[];
  const context={selectedID:{value:'task-a'},resultOffset:{value:0},results:{value:[]},nextResultOffset:{value:-1},resultRequest:0,stopped:false,showError:e=>{throw e},api:{get:()=>new Promise((resolve,reject)=>pending.push({resolve,reject}))}};
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
