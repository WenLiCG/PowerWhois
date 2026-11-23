import { defineStore } from "pinia";

export const useSettingStore = defineStore("setting", {
    state: () => ({
        webCheckDomainLimit: 100,
        typoDefaultCcTlds: [],
        registerApis: [],
        whoisApis: []
    }),

    actions: {
        setSetting(newSetting) {
            this.webCheckDomainLimit = newSetting.webCheckDomainLimit || 100;
            for (const key of ['typoDefaultCcTlds', 'registerApis', 'whoisApis']) {
                this[key] = Array.isArray(newSetting[key]) ? newSetting[key] : [];
            }
        },
        updateSetting(newSetting) {
            this.setSetting({ ...newSetting,
                registerApis: (newSetting.registerApis || []).map(api => api.apiName),
                whoisApis: (newSetting.whoisApis || []).map(api => api.apiName)
            });
        },
        clearSetting() {
            this.webCheckDomainLimit = 100;
            this.typoDefaultCcTlds = [];
            this.registerApis = [];
            this.whoisApis = [];
        }
    }
});
