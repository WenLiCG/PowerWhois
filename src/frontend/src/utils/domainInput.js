export function normalizeDomainInput(value) {
    return (value || "").trim().replace(/[。．｡]/g, ".").replace(/([^.])\.$/, "$1").toLowerCase();
}

export function isPlainDomainInput(value) {
    return value.includes(".") && !/[\s/:?#]/.test(value) && value.split(".").every(Boolean);
}
