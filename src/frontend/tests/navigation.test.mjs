import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const page = readFileSync(new URL("../src/pages/IndexPage.vue", import.meta.url), "utf8");

test("login returns to the requested protected page and changing credentials reopens settings after login", () => {
    const tab = { value: "webCheck" }, loginNotice = { value: "" }, tokenStore = { token: null }, sockets = [];
    const app = vm.createContext({
        tab, loginNotice, tokenStore, afterLogin: "webCheck", getUsername() {}, getWebSettings() {}, $q: { notify() {} },
        close: () => sockets.push("closed"), open: () => sockets.push("opened"), removeToken: () => { tokenStore.token = null; }
    });
    vm.runInContext(page.slice(page.indexOf("function selectTab("), page.indexOf("watch(")), app);
    vm.runInContext(page.slice(page.indexOf("function credentialsChanged("), page.indexOf("async function logout(")), app);

    app.selectTab("bulkCheck");
    assert.equal(tab.value, "login");
    assert.match(loginNotice.value, /批量任务/);
    tokenStore.token = "first-session";
    app.onLogin();
    assert.equal(tab.value, "bulkCheck");
    app.credentialsChanged();
    assert.equal(tokenStore.token, null);
    assert.deepEqual(sockets, ["closed", "opened"]);
    assert.equal(tab.value, "login");
    assert.match(loginNotice.value, /凭据/);
    tokenStore.token = "new-session";
    app.onLogin();
    assert.equal(tab.value, "setting");
    assert.equal(loginNotice.value, "");
});
