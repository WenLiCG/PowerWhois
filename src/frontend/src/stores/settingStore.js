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
            this.webCheckDomainLimit = newSetting.webCheckDomainLimit;
            this.typoDefaultCcTlds = newSetting.typoDefaultCcTlds || [];
            this.registerApis = newSetting.registerApis;
            this.whoisApis = newSetting.whoisApis;
        },
        updateSetting(newSetting) {
            if (newSetting.webCheckDomainLimit) {
                this.webCheckDomainLimit = newSetting.webCheckDomainLimit;
            }

            if (newSetting.typoDefaultCcTlds) {
                this.typoDefaultCcTlds = newSetting.typoDefaultCcTlds || [];
            }

            this.registerApis = [];
            if (newSetting.registerApis && newSetting.registerApis.length > 0) {
                newSetting.registerApis.forEach((apiItem) => {
                    this.registerApis.push(apiItem.apiName);
                });
            }

            this.whoisApis = [];
            if (newSetting.whoisApis && newSetting.whoisApis.length > 0) {
                newSetting.whoisApis.forEach((apiItem) => {
                    this.whoisApis.push(apiItem.apiName);
                });
            }
        },
        clearSetting() {
            this.webCheckDomainLimit = 100;
            this.typoDefaultCcTlds = [];
            this.registerApis = [];
            this.whoisApis = [];
        }
    }
});
