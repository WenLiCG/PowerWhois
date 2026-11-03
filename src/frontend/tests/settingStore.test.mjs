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
    store.updateSetting({typoDefaultCcTlds: [{tld: 'com', isSelected: true}], whoisApis: [{apiName: 'test'}]});
    assert.deepEqual(store.whoisApis, ['test']);
    store.updateSetting({typoDefaultCcTlds: null});
    assert.deepEqual(store.typoDefaultCcTlds, []);
});
