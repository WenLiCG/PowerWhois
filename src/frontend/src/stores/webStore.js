import { defineStore } from "pinia";
import { date } from "quasar";

export const useWebStore = defineStore("web", {
    state: () => ({
        domains: [],
        byName: {},
        submitting: false,
        resetOnReply: false,
        unCheckDomains: 0,
        unRegisterDomains: 0
    }),
    getters: {
        checking: (state) => state.submitting || state.unCheckDomains > 0,
        checkedTakenDomains: (state) => state.domains.filter((row) => row.status === "Taken").length,
        checkedFreeDomains: (state) => state.domains.filter((row) => row.status === "Free").length,
        checkedErrorDomains: (state) => state.domains.filter((row) => row.status === "Error").length,
        checkedUnknownDomains: (state) => state.domains.filter((row) => row.status === "Unknown" || row.status === "Reserved").length
    },
    actions: {
        beginCheck(recheck = false) {
            this.submitting = true;
            this.resetOnReply = !recheck;
        },
        setCheckingDomains(newDomains, checkType, typoType) {
            if (this.resetOnReply) {
                this.resetOnReply = false;
                this.domains = [];
                this.byName = {};
                this.unCheckDomains = 0;
            }
            for (const domain of newDomains) {
                const row = this.byName[domain];
                if (row) {
                    if (!row.isChecked) continue;
                    Object.assign(row, { isChecked: false, status: null, errorInfo: "", checkedRawResponse: "" });
                } else {
                    this.domains.push({ domainID: this.domains.length + 1, domain, checkType, typoType, isChecked: false });
                    this.byName[domain] = this.domains[this.domains.length - 1];
                }
                this.unCheckDomains++;
            }
        },
        updateCheckResult(result) {
            const row = this.byName[result.domain];
            if (!row || row.isChecked) return;
            Object.assign(row, {
                isChecked: true,
                status: result.registerStatus,
                lookupType: result.lookupType,
                errorInfo: result.queryError,
                checkedRawResponse: result.rawResponse,
                createdDate: result.createdDate ? date.formatDate(result.createdDate.slice(0, 10), "YYYY-MM-DD") : "",
                expiryDate: result.expiryDate ? date.formatDate(result.expiryDate.slice(0, 10), "YYYY-MM-DD") : "",
                domainStatus: result.domainStatus,
                nameServer: result.nameServer?.length ? result.nameServer : ["N/A"]
            });
            this.unCheckDomains--;
        },
        finishCheck() {
            this.submitting = false;
            this.resetOnReply = false;
            for (const row of this.domains) {
                if (!row.isChecked) {
                    Object.assign(row, { isChecked: true, status: "Error", errorInfo: "查询中断，可重新查询" });
                }
            }
            this.unCheckDomains = 0;
        },
        setRegisteringDomains(domains, operationId) {
            for (const name of domains) {
                const row = this.byName[name];
                Object.assign(row, { selectedRegister: true, registerStatus: null, registerRawResponse: "", operationId });
            }
            this.unRegisterDomains = domains.length;
        },
        updateRegisterResult(result) {
            const row = this.byName[result.domainName];
            if (!row?.selectedRegister) return;
            if (!row.registerStatus) this.unRegisterDomains--;
            row.registerStatus = result.registerStatus;
            row.registerRawResponse = result.rawResponse;
            row.operationId = result.operationId;
        },
        finishRegistration(rejected = false) {
            for (const row of this.domains) {
                if (row.selectedRegister && !row.registerStatus) {
                    row.registerStatus = rejected ? "error" : "outcome_unknown";
                    row.registerRawResponse = rejected ? "提交请求被拒绝" : "连接中断，请用操作编号向注册商核对。";
                }
            }
            this.unRegisterDomains = 0;
        },
        clearDomains() {
            this.domains = [];
            this.byName = {};
            this.unCheckDomains = 0;
            this.unRegisterDomains = 0;
            this.submitting = false;
            this.resetOnReply = false;
        }
    }
});
