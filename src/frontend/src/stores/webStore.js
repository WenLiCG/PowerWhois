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
        isErrorRecheck: false
    }),

    actions: {
        setIsErrorRecheck(isErrorRecheck) {
            this.isErrorRecheck = isErrorRecheck;
        },
        setCheckingDomains(newDomains, checkType, typoType) {
            if (this.isErrorRecheck) {
                newDomains.forEach((domain) => {
                    for (let i = 0; i < this.domains.length; i++) {
                        if (this.domains[i].domain == domain.toLowerCase()) {
                            this.domains[i].lookupType = null;
                            this.domains[i].status = null;
                            this.domains[i].errorInfo = "";
                            this.domains[i].createdDate = null;
                            this.domains[i].expiryDate = null;
                            this.domains[i].nameServer = null;
                            this.domains[i].domainStatus = null;
                            this.domains[i].checkedRawResponse = null;
                            this.domains[i].isChecked = false;
                            this.domains[i].selectedRegister = false;
                            this.domains[i].registerStatus = null;
                            this.domains[i].registerRawResponse = null;
                            this.unCheckDomains++;
                            this.recount();
                        }
                    }
                });
            } else {
                let domainID = this.domains.length + 1;
                newDomains.forEach((domain) => {
                    this.domains.push({
                        domainID: domainID,
                        domain: domain.toLowerCase(),
                        checkType: checkType,
                        typoType: typoType,
                        lookupType: null,
                        status: null,
                        errorInfo: "",
                        createdDate: null,
                        expiryDate: null,
                        nameServer: null,
                        domainStatus: null,
                        checkedRawResponse: null,
                        isChecked: false,
                        selectedRegister: false,
                        registerStatus: null,
                        registerRawResponse: null
                    });
                    domainID++;
                    this.unCheckDomains++;
                });
            }
        },
        updateCheckResult(domainResult) {
            for (let i = 0; i < this.domains.length; i++) {
                if (this.domains[i].domain == domainResult.domain.toLowerCase() && !this.domains[i].isChecked) {
                    this.domains[i].status = domainResult.registerStatus;
                    this.domains[i].lookupType = domainResult.lookupType;
                    this.domains[i].checkedRawResponse = [domainResult.source && `来源: ${domainResult.source}`, domainResult.queriedAt && `查询时间: ${domainResult.queriedAt}`, domainResult.queryError, domainResult.rawResponse].filter(Boolean).join("\n");
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
            this.unCheckDomains = this.domains.filter(d => !d.isChecked).length;
            this.checkedTakenDomains = this.domains.filter(d => d.status === 'Taken').length;
            this.checkedFreeDomains = this.domains.filter(d => ['NotFound','Free'].includes(d.status)).length;
            this.checkedUnknownDomains = this.domains.filter(d => d.status === 'Unknown').length;
            this.checkedTimeoutDomains = this.domains.filter(d => d.status === 'Timeout').length;
            this.checkedErrorDomains = this.domains.filter(d => d.status === 'Error').length;
        },
        interruptQueries() {
            for (const d of this.domains) {
                if (!d.isChecked) { d.status = 'Error'; d.errorInfo = '连接中断，请重查'; d.isChecked = true; }
            }
            this.recount();
        },
        clearDomains() {
            this.domains = [];
            this.unCheckDomains = 0;
            this.unRegisterDomains = 0;
            this.checkedTakenDomains = 0;
            this.checkedFreeDomains = 0;
            this.checkedUnknownDomains = 0;
            this.checkedTimeoutDomains = 0;
            this.checkedErrorDomains = 0;
            this.isErrorRecheck = false;
        }
    }
});
