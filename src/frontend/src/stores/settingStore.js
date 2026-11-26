import { defineStore } from "pinia";

export const useSettingStore = defineStore("setting", {
    state: () => ({
        webCheckDomainLimit: 100,
        typoDefaultCcTlds: [],
        registerApis: [],
        whoisApis: [],
        proxyConfigured: false
    }),

    actions: {
        setSetting(newSetting) {
            this.webCheckDomainLimit = newSetting.webCheckDomainLimit || 100;
            for (const key of ["typoDefaultCcTlds", "registerApis", "whoisApis"]) {
                this[key] = Array.isArray(newSetting[key]) ? newSetting[key] : [];
            }
            if (typeof newSetting.proxyConfigured === "boolean") {
                this.proxyConfigured = newSetting.proxyConfigured;
            } else if ("socketProxyHost" in newSetting || "socketProxyPort" in newSetting) {
                this.proxyConfigured = Boolean(String(newSetting.socketProxyHost || "").trim() && Number(newSetting.socketProxyPort) > 0);
            }
        },
        updateSetting(newSetting) {
            this.setSetting({
                ...newSetting,
                registerApis: (newSetting.registerApis || []).map((api) => api.apiName),
                whoisApis: (newSetting.whoisApis || []).map((api) => api.apiName),
                proxyConfigured: Boolean(String(newSetting.socketProxyHost || "").trim() && Number(newSetting.socketProxyPort) > 0)
            });
        },
        clearSetting() {
            this.webCheckDomainLimit = 100;
            this.typoDefaultCcTlds = [];
            this.registerApis = [];
            this.whoisApis = [];
            this.proxyConfigured = false;
        }
    }
});
