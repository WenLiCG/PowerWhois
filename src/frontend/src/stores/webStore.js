import { defineStore } from "pinia";
import { date } from "quasar";

export const useWebStore = defineStore("web", {
    state: () => ({
        domains: [],
        unCheckDomains: 0,
        unRegisterDomains: 0,
        checkedTakenDomains: 0,
        checkedFreeDomains: 0,
        checkedUnknownDomains: 0,
        checkedTimeoutDomains: 0,
        checkedErrorDomains: 0,
        pendingQuery: null,
        // Tab whose rows a pending retry replaces; both tabs send retries as webCheck events.
        pendingScope: null,
        pendingMode: null,
        // Query modes behind each tab's current rows, so result columns do not follow the live selector.
        resultModes: { webCheck: [], typoCheck: [] },
        // Preserve draft inputs and mode selections across tab switches.
        webDomainInput: "",
        webQueryType: "whoisQuery",
        typoDomainInput: "",
        typoQueryType: "whoisQuery",
        // Row IDs stay unique across tabs because registration selects rows by ID.
        nextDomainID: 1
    }),

    getters: {
        busy: (state) => !!state.pendingQuery || state.unCheckDomains > 0 || state.unRegisterDomains > 0,
        // The tab that owns the running query or registration; the other tab explains why it is locked.
        activeCheckType: (state) => {
            if (state.pendingQuery === "retry") return state.pendingScope;
            if (state.pendingQuery === "webCheck" || state.pendingQuery === "typoCheck") return state.pendingQuery;
            const row = state.domains.find((d) => !d.isChecked || (d.selectedRegister && d.registerStatus == null));
            return row ? row.checkType : null;
        }
    },

    actions: {
        rememberMode(checkType, mode) {
            if (!mode || !this.resultModes[checkType]) return;
            if (!this.resultModes[checkType].includes(mode)) this.resultModes[checkType].push(mode);
        },
        setCheckingDomains(newDomains, checkType, typoType) {
            if (!newDomains.length) return;
            if (this.pendingQuery === "retry") {
                const scope = this.pendingScope;
                const retryDomains = new Set(newDomains.map((domain) => domain.toLowerCase()));
                for (const row of this.domains) {
                    if ((scope && row.checkType !== scope) || !retryDomains.has(row.domain)) continue;
                    row.lookupType = null;
                    row.viaProxy = false;
                    row.source = "";
                    row.queriedAt = "";
                    row.rawResponse = "";
                    row.status = null;
                    row.errorInfo = "";
                    row.createdDate = null;
                    row.expiryDate = null;
                    row.nameServer = null;
                    row.domainStatus = null;
                    row.isChecked = false;
                }
                if (scope) this.rememberMode(scope, this.pendingMode);
                this.pendingQuery = null;
                this.pendingScope = null;
                this.recount();
            } else {
                // A new query replaces only its own tab's results; the other tab keeps its evidence.
                if (this.pendingQuery === checkType) {
                    this.clearDomains(checkType);
                    this.rememberMode(checkType, this.pendingMode);
                }
                const existingByDomain = new Map(this.domains.filter((row) => row.checkType === checkType).map((row) => [row.domain, row]));
                newDomains.forEach((domain) => {
                    const normalized = domain.toLowerCase();
                    const existing = existingByDomain.get(normalized);
                    if (existing) {
                        if (typoType) {
                            if (!Array.isArray(existing.typoTypes)) {
                                existing.typoTypes = existing.typoType ? [existing.typoType] : [];
                            }
                            if (!existing.typoTypes.includes(typoType)) {
                                existing.typoTypes.push(typoType);
                            }
                        }
                        return;
                    }
                    const created = {
                        domainID: this.nextDomainID++,
                        domain: normalized,
                        checkType: checkType,
                        typoType: typoType,
                        typoTypes: typoType ? [typoType] : [],
                        lookupType: null,
                        viaProxy: false,
                        source: "",
                        queriedAt: "",
                        rawResponse: "",
                        status: null,
                        errorInfo: "",
                        createdDate: null,
                        expiryDate: null,
                        nameServer: null,
                        domainStatus: null,
                        isChecked: false,
                        selectedRegister: false,
                        registerStatus: null,
                        registerRawResponse: null
                    };
                    this.domains.push(created);
                    existingByDomain.set(normalized, created);
                    this.unCheckDomains++;
                });
            }
        },
        updateCheckResult(domainResult) {
            for (let i = 0; i < this.domains.length; i++) {
                if (this.domains[i].domain == domainResult.domain.toLowerCase() && !this.domains[i].isChecked) {
                    this.domains[i].status = domainResult.registerStatus;
                    this.domains[i].lookupType = domainResult.lookupType;
                    this.domains[i].viaProxy = Boolean(domainResult.viaProxy);
                    this.domains[i].source = domainResult.source || "";
                    this.domains[i].queriedAt = domainResult.queriedAt || "";
                    this.domains[i].rawResponse = domainResult.rawResponse || "";
                    this.domains[i].errorInfo = domainResult.queryError;

                    if (domainResult.registerStatus == "Taken") {
                        if (domainResult.lookupType == "whois" || domainResult.lookupType == "rdap") {
                            this.domains[i].createdDate = domainResult.createdDate ? date.formatDate(domainResult.createdDate, "YYYY-MM-DD") : "—";
                            this.domains[i].expiryDate = domainResult.expiryDate ? date.formatDate(domainResult.expiryDate, "YYYY-MM-DD") : "—";
                            this.domains[i].domainStatus = domainResult.domainStatus;
                        }

                        if (domainResult.nameServer && domainResult.nameServer.length > 0) {
                            this.domains[i].nameServer = domainResult.nameServer.map((item) => item.toLowerCase());
                        } else {
                            this.domains[i].nameServer = ["N/A"];
                        }

                        this.checkedTakenDomains++;
                    } else if (["NotFound", "Free"].includes(domainResult.registerStatus)) {
                        this.checkedFreeDomains++;
                    } else if (domainResult.registerStatus == "Unknown") {
                        this.checkedUnknownDomains++;
                    } else if (domainResult.registerStatus == "Timeout") {
                        this.checkedTimeoutDomains++;
                    } else if (domainResult.registerStatus == "Error") {
                        this.domains[i].errorInfo = domainResult.queryError;
                        this.checkedErrorDomains++;
                    }

                    this.domains[i].isChecked = true;
                    this.unCheckDomains--;
                }
            }
        },
        setRegisteringDomains(domains) {
            domains.forEach((domain) => {
                for (let i = 0; i < this.domains.length; i++) {
                    if (this.domains[i].domainID == domain.domainID) {
                        this.domains[i].selectedRegister = true;
                        this.domains[i].registerStatus = null;
                        this.domains[i].registerRawResponse = null;
                        this.unRegisterDomains++;
                    }
                }
            });
        },
        updateRegisterResult(registerResult) {
            for (let i = 0; i < this.domains.length; i++) {
                if (this.domains[i].domain == registerResult.domainName && this.domains[i].selectedRegister) {
                    if (this.domains[i].registerStatus == null) {
                        this.domains[i].registerStatus = registerResult.registerStatus;
                        this.domains[i].registerRawResponse = registerResult.rawResponse;
                        this.unRegisterDomains--;
                    }
                }
            }
        },
        recount() {
            this.unCheckDomains = this.domains.filter((d) => !d.isChecked).length;
            this.checkedTakenDomains = this.domains.filter((d) => d.status === "Taken").length;
            this.checkedFreeDomains = this.domains.filter((d) => ["NotFound", "Free"].includes(d.status)).length;
            this.checkedUnknownDomains = this.domains.filter((d) => d.status === "Unknown").length;
            this.checkedTimeoutDomains = this.domains.filter((d) => d.status === "Timeout").length;
            this.checkedErrorDomains = this.domains.filter((d) => d.status === "Error").length;
        },
        finishPendingRegistrations(status, message) {
            for (const domain of this.domains) {
                if (domain.selectedRegister && domain.registerStatus == null) {
                    domain.registerStatus = status;
                    domain.registerRawResponse = message;
                }
            }
            this.unRegisterDomains = 0;
        },
        stopQueries() {
            for (const d of this.domains) {
                if (!d.isChecked) {
                    d.status = "Unknown";
                    d.errorInfo = "已停止查询，可点击重查";
                    d.isChecked = true;
                }
            }
            this.pendingQuery = null;
            this.pendingScope = null;
            this.recount();
        },
        interruptQueries() {
            this.finishPendingRegistrations("outcome_unknown", "连接中断，无法确认是否已提交。请先向注册商核实订单。");
            for (const d of this.domains) {
                if (!d.isChecked) {
                    d.status = "Error";
                    d.errorInfo = "连接中断，请重查";
                    d.isChecked = true;
                }
            }
            this.pendingQuery = null;
            this.pendingScope = null;
            this.recount();
        },
        // Without a tab, clear everything (logout); with one, keep the other tab's results.
        clearDomains(checkType = null) {
            this.domains = checkType ? this.domains.filter((row) => row.checkType !== checkType) : [];
            if (checkType) this.resultModes[checkType] = [];
            else {
                this.resultModes = { webCheck: [], typoCheck: [] };
                this.webDomainInput = "";
                this.typoDomainInput = "";
                this.webQueryType = "whoisQuery";
                this.typoQueryType = "whoisQuery";
            }
            this.unRegisterDomains = this.domains.filter((row) => row.selectedRegister && row.registerStatus == null).length;
            this.pendingQuery = null;
            this.pendingScope = null;
            this.recount();
        }
    }
});
