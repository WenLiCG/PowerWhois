import test from 'node:test';
import assert from 'node:assert/strict';
import {createPinia,setActivePinia} from 'pinia';
import {useWebStore} from '../src/stores/webStore.js';
import {rules} from '../src/boot/rules.js';
import {buildWebResultsCsv,sanitizeCsvCell} from '../src/utils/exportCsv.js';
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

test('disconnect preserves finished registrations and marks unresolved orders for verification', () => {
  setActivePinia(createPinia());
  const store = useWebStore();
  store.setCheckingDomains(['one.com', 'two.com'], 'webCheck', '');
  store.setRegisteringDomains(store.domains);
  store.updateRegisterResult({domainName: 'one.com', registerStatus: 'success', rawResponse: 'confirmed'});
  store.interruptQueries();
  assert.deepEqual(store.domains.map(d => d.registerStatus), ['success', 'outcome_unknown']);
  assert.equal(store.domains[0].registerRawResponse, 'confirmed');
  assert.match(store.domains[1].registerRawResponse, /核实订单/);
  assert.equal(store.unRegisterDomains, 0);
  store.setIsErrorRecheck(true);
  store.setCheckingDomains(['ONE.COM', 'one.com', 'two.com'], 'webCheck', '');
  assert.equal(store.unCheckDomains, 2);
  assert.deepEqual(store.domains.map(d => d.registerStatus), ['success', 'outcome_unknown']);
});

test('typo duplicate candidates across groups merge without inflating counts or sticking recheck flag', () => {
  setActivePinia(createPinia());
  const store = useWebStore();
  store.setCheckingDomains(['google.com', 'gogle.com'], 'typoCheck', 'reverseLetter');
  store.setCheckingDomains(['GOOGLE.COM', 'google.net'], 'typoCheck', 'wrongTlds');
  assert.equal(store.domains.length, 3);
  assert.equal(store.unCheckDomains, 3);
  assert.deepEqual(store.domains[0].typoTypes, ['reverseLetter', 'wrongTlds']);

  store.updateCheckResult({domain: 'google.com', registerStatus: 'Taken', lookupType: 'rdap'});
  assert.equal(store.checkedTakenDomains, 1);
  assert.equal(store.unCheckDomains, 2);

  store.stopQueries();
  assert.equal(store.unCheckDomains, 0);
  assert.equal(store.checkedUnknownDomains, 2);

  store.setIsErrorRecheck(true);
  store.setCheckingDomains(['gogle.com'], 'webCheck', '');
  assert.equal(store.isErrorRecheck, false);
  assert.equal(store.unCheckDomains, 1);
});

test('web CSV export escapes formulas and preserves merged typo labels', () => {
  assert.equal(sanitizeCsvCell('=SUM(A1)'), "'=SUM(A1)");
  assert.equal(sanitizeCsvCell('  +cmd'), "'  +cmd");
  const csv = buildWebResultsCsv([
    {domain: 'example.com', typoTypes: ['reverseLetter', 'wrongTlds'], status: 'Taken', lookupType: 'rdap', errorInfo: '=injected'}
  ], true);
  assert.match(csv, /^\ufeffDomain,Typo Type,Status/);
  assert.match(csv, /reverseLetter\/wrongTlds/);
  assert.match(csv, /'=injected/);
});
