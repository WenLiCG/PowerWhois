import assert from "node:assert/strict";
import { test } from "node:test";
import { createPinia, setActivePinia } from "pinia";
import { useWebStore } from "../src/stores/webStore.js";

test("disconnect preserves results and lets the user retry pending rows", () => {
    setActivePinia(createPinia());
    const store = useWebStore();
    store.beginCheck();
    store.setCheckingDomains(["example.com", "example.net"], "webCheck", "");
    store.updateCheckResult({ domain: "example.com", registerStatus: "Taken", rawResponse: "sample" });
    store.finishCheck();
    assert.equal(store.checking, false);
    assert.equal(store.byName["example.com"].checkedRawResponse, "sample");
    assert.equal(store.byName["example.net"].status, "Error");
    store.beginCheck(true);
    store.setCheckingDomains(["example.net"], "webCheck", "");
    store.updateCheckResult({ domain: "example.net", registerStatus: "Unknown" });
    store.finishCheck();
    assert.equal(store.domains.length, 2);
    assert.equal(store.checkedErrorDomains, 0);
    assert.equal(store.checkedUnknownDomains, 1);
});

test("disconnect keeps the registration reference for manual reconciliation", () => {
    setActivePinia(createPinia());
    const store = useWebStore();
    store.beginCheck();
    store.setCheckingDomains(["example.com"], "webCheck", "");
    store.updateCheckResult({ domain: "example.com", registerStatus: "Free" });
    store.finishCheck();
    store.setRegisteringDomains(["example.com"], "fixture-operation");
    store.finishRegistration();
    assert.equal(store.unRegisterDomains, 0);
    assert.equal(store.byName["example.com"].registerStatus, "outcome_unknown");
    assert.equal(store.byName["example.com"].operationId, "fixture-operation");
});
