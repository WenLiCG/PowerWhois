export function sanitizeCsvCell(value) {
    const str = value == null ? "" : String(value);
    const trimmed = str.replace(/^ +/, "");
    if (trimmed.length > 0 && "=+-@\t\r\n".includes(trimmed[0])) {
        return "'" + str;
    }
    return str;
}

function encodeCsvValue(value) {
    const safe = sanitizeCsvCell(value);
    if (/[",\r\n]/.test(safe)) {
        return `"${safe.replace(/"/g, '""')}"`;
    }
    return safe;
}

export function buildWebResultsCsv(rows, includeTypo = false) {
    const headers = ["Domain", ...(includeTypo ? ["Typo Type"] : []), "Status", "Lookup Type", "Created Date", "Expiry Date", "Domain Status", "Name Server", "Error Info"];
    const lines = [headers.join(",")];
    for (const row of rows) {
        const ns = Array.isArray(row.nameServer) ? row.nameServer.filter((n) => n && n !== "N/A").join(",") : "";
        const fields = [
            row.domain || "",
            ...(includeTypo ? [Array.isArray(row.typoTypes) && row.typoTypes.length ? row.typoTypes.join("/") : row.typoType || ""] : []),
            row.status || "Pending",
            row.lookupType || "",
            row.createdDate && row.createdDate !== "—" ? row.createdDate : "",
            row.expiryDate && row.expiryDate !== "—" ? row.expiryDate : "",
            row.domainStatus || "",
            ns,
            row.errorInfo || ""
        ];
        lines.push(fields.map(encodeCsvValue).join(","));
    }
    return "\ufeff" + lines.join("\n") + "\n";
}

export function downloadCsvContent(csvText, filename) {
    const url = URL.createObjectURL(new Blob([csvText], { type: "text/csv;charset=utf-8" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    setTimeout(() => {
        document.body.removeChild(link);
        URL.revokeObjectURL(url);
    }, 500);
}
