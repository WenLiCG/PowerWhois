// One confirmation submits several domains under the same operationId, so a
// pending row is identified by both values together.
export function pendingRowKey(row) {
    return `${row.operationId}\u0000${row.domainName}`;
}

// Returns rows without the reconciled one; other domains of the same batch stay.
export function withoutPendingRow(rows, row) {
    const key = pendingRowKey(row);
    return rows.filter((item) => pendingRowKey(item) !== key);
}
