import { defineStore } from "pinia";

export const useBulkStore = defineStore("bulk", {
    state: () => ({
        bulkCheckQueryType: null,
        bulkCheckStatus: null,
        runingProgress: 0,
        runingProgressPercent: "0 %",
        bulkCheckInfo: [
            {
                label: "任务状态",
                header: "root",
                id: "root",
                icon: "fa-solid fa-bars-progress",
                color: "primary",
                total: 0,
                done: 0,
                children: [
                    {
                        label: "已完成任务",
                        id: "done",
                        value: 0,
                        icon: "fa-solid fa-check-double",
                        color: "positive",
                        children: [
                            {
                                label: "已注册",
                                id: "taken",
                                value: 0,
                                icon: "fa-regular fa-circle-check",
                                color: "warning"
                            },
                            {
                                label: "未注册",
                                id: "free",
                                value: 0,
                                icon: "fa-solid fa-ban",
                                color: "secondary"
                            },
                            {
                                label: "错误",
                                id: "error",
                                value: 0,
                                icon: "fa-solid fa-triangle-exclamation",
                                color: "negative"
                            }
                        ]
                    },
                    {
                        label: "剩余任务: ",
                        id: "left",
                        value: 0,
                        icon: "fa-regular fa-hourglass-half",
                        color: "accent"
                    }
                ]
            }
        ]
    }),

    actions: {
        setBulkCheckStatus(status) {
            this.bulkCheckStatus = status;
        },

        setBulkCheckQueryType(queryType) {
            switch (queryType) {
                case "whoisQuery":
                    this.bulkCheckQueryType = "Whois";
                    break;
                case "whoisQueryWithProxy":
                    this.bulkCheckQueryType = "Whois + Proxy";
                    break;
                case "dnsQuery":
                    this.bulkCheckQueryType = "DNS";
                    break;
                case "mixedQuery":
                    this.bulkCheckQueryType = "Mixed";
                    break;
                default:
                    this.bulkCheckQueryType = queryType;
                    break;
            }
        },

        updateBulkCheckInfo(info) {
            let doneDomains = info.TakenDomains + info.FreeDomains + info.ErrorDomains;
            if (info.TotalDomains > 0) {
                this.runingProgress = parseFloat((doneDomains / info.TotalDomains).toFixed(4));
                this.runingProgressPercent = (this.runingProgress * 100).toFixed(2) + " %";
            } else {
                this.runingProgress = 0;
                this.runingProgressPercent = "0 %";
            }
            this.bulkCheckInfo[0].total = info.TotalDomains;
            this.bulkCheckInfo[0].done = doneDomains;
            this.bulkCheckInfo[0].children[0].value = doneDomains;
            this.bulkCheckInfo[0].children[0].children[0].value = info.TakenDomains;
            this.bulkCheckInfo[0].children[0].children[1].value = info.FreeDomains;
            this.bulkCheckInfo[0].children[0].children[2].value = info.ErrorDomains;
            this.bulkCheckInfo[0].children[1].value = info.RemainDomains;
        },

        clearStatusAndInfo() {
            this.bulkCheckStatus = null;
            this.runingProgress = 0;
            this.runingProgressPercent = "0 %";
            this.bulkCheckInfo[0].total = 0;
            this.bulkCheckInfo[0].done = 0;
            this.bulkCheckInfo[0].children[0].value = 0;
            this.bulkCheckInfo[0].children[0].children[0].value = 0;
            this.bulkCheckInfo[0].children[0].children[1].value = 0;
            this.bulkCheckInfo[0].children[0].children[2].value = 0;
            this.bulkCheckInfo[0].children[1].value = 0;
        }
    },

    getters: {
        errorDomainsCount: (state) => state.bulkCheckInfo[0].children[0].children[2].value
    }
});
