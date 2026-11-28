import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
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

test('saving settings sends only an explicitly selected new password and asks for login after success', async () => {
    const source = readFileSync(new URL('../src/components/AdminSetting.vue', import.meta.url), 'utf8');
    const code = source.slice(source.indexOf('function splitNonEmptyLines('), source.indexOf('onMounted(() =>'));
    const settings = { value: { authUsername: 'tester', authPassword: 'unused', secretActions: {authPassword: 'keep'} } };
    const payloads = [];
    const events = [];
    let fail = false;
    const context = {
        settings, savedUsername: 'tester', savedProxyHost: '', submitting: {value: false}, loadingSettings: {value: false},
        ccTldOptions: {value: []}, selectedCcTlds: {value: []}, is: {number: Number.isFinite},
        confirmProxyPasswordReuse: async () => true, settingStore: {updateSetting() {}}, $q: {notify() {}},
        emit: (event) => events.push(event),
        api: {async put(_, payload) {
            payloads.push(payload);
            if (fail) throw Error('unavailable');
            return {data: {authUsername: 'tester'}};
        }}
    };
    vm.runInNewContext(code, context);
    await context.onSubmit();
    assert.equal(payloads[0].authPassword, '');
    assert.equal(payloads[0].secretActions.authPassword, 'keep');
    assert.deepEqual(events, []);

    settings.value.authPassword = ' x ';
    settings.value.secretActions.authPassword = 'set';
    fail = true;
    await context.onSubmit();
    assert.equal(settings.value.authPassword, ' x ');
    assert.deepEqual(events, []);

    fail = false;
    await context.onSubmit();
    assert.equal(payloads[2].authPassword, ' x ');
    assert.equal(payloads[2].secretActions.authPassword, 'set');
    assert.equal(settings.value.authPassword, '');
    assert.deepEqual(events, ['credentials-changed']);
});
