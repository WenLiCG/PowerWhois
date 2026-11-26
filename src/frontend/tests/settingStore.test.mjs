import test from 'node:test';
import assert from 'node:assert/strict';
import { createPinia, setActivePinia } from 'pinia';
import { useSettingStore } from '../src/stores/settingStore.js';

test('empty server configuration stays usable by every selector', () => {
    setActivePinia(createPinia());
    const store = useSettingStore();
    store.setSetting({typoDefaultCcTlds: null, registerApis: null});
    assert.deepEqual(store.typoDefaultCcTlds, []);
    assert.deepEqual(store.whoisApis, []);
    store.updateSetting({typoDefaultCcTlds: [{tld: 'com', isSelected: true}], whoisApis: [{apiName: 'test'}], socketProxyHost: 'brd.superproxy.io', socketProxyPort: 22228});
    assert.deepEqual(store.whoisApis, ['test']);
    assert.equal(store.proxyConfigured, true);
    store.updateSetting({typoDefaultCcTlds: null, socketProxyHost: '', socketProxyPort: 0});
    assert.deepEqual(store.typoDefaultCcTlds, []);
    assert.equal(store.proxyConfigured, false);
    store.setSetting({proxyConfigured: true});
    assert.equal(store.proxyConfigured, true);
    store.clearSetting();
    assert.equal(store.proxyConfigured, false);
});
