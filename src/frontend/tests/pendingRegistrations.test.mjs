import assert from "node:assert/strict";
import { test } from "node:test";
import { pendingRowKey, withoutPendingRow } from "../src/utils/pendingRegistrations.js";

// Verification finding F4: one batch shares an operationId across domains.
test("reconciling one domain keeps the batch's other pending domains", () => {
    const batch = "c773af68-facd-4250-bdab-c57eedb246fc";
    const rows = [
        { domainName: "example.com", operationId: batch },
        { domainName: "example.net", operationId: batch },
        { domainName: "example.com", operationId: "4ad0f7c1-8a37-4c1e-9d1b-3f6b5d0b8e21" }
    ];
    const left = withoutPendingRow(rows, rows[0]);
    assert.deepEqual(
        left.map((row) => [row.domainName, row.operationId]),
        [
            ["example.net", batch],
            ["example.com", "4ad0f7c1-8a37-4c1e-9d1b-3f6b5d0b8e21"]
        ]
    );
});

test("row keys are unique per domain and operation", () => {
    const keys = new Set(
        [
            { domainName: "example.com", operationId: "a" },
            { domainName: "example.net", operationId: "a" },
            { domainName: "example.com", operationId: "b" }
        ].map(pendingRowKey)
    );
    assert.equal(keys.size, 3);
});
