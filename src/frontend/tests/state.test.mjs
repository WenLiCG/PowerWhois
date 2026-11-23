import test from 'node:test';
import assert from 'node:assert/strict';
import {createPinia,setActivePinia} from 'pinia';
import {useWebStore} from '../src/stores/webStore.js';
import {rules} from '../src/boot/rules.js';
test('duplicate responses never corrupt counts; disconnect and retry remain usable',()=>{
 setActivePinia(createPinia());const s=useWebStore();s.setCheckingDomains(['example.com','example.org'],'webCheck','');
 const r={domain:'example.com',registerStatus:'NotFound',status:'not_found',lookupType:'rdap',queriedAt:'2026-11-03T00:00:00Z',source:'fixture'};
 s.updateCheckResult(r);s.updateCheckResult(r);assert.equal(s.checkedFreeDomains,1);assert.equal(s.unCheckDomains,1);
 s.interruptQueries();assert.equal(s.unCheckDomains,0);assert.equal(s.checkedErrorDomains,1);
 s.setIsErrorRecheck(true);s.setCheckingDomains(['example.org'],'webCheck','');assert.equal(s.unCheckDomains,1);assert.equal(s.checkedErrorDomains,0);
 s.updateCheckResult({...r,domain:'example.org',registerStatus:'Unknown',status:'unknown'});assert.equal(s.checkedUnknownDomains,1);assert.equal(s.unCheckDomains,0);
 s.clearDomains();assert.equal(s.checkedUnknownDomains,0);
});
test('native Quasar rule adapter accepts zero and rejects empty numeric values',()=>{
 assert.equal(rules.required('required')(0),true);assert.equal(rules.required('required')(' '),'required');
 assert.equal(rules.numeric('number')(''),'number');assert.equal(rules.numeric('number')('abc'),'number');
 assert.equal(rules.minValue(1,'min')(0),'min');assert.equal(rules.maxValue(5,'max')(6),'max');
});
