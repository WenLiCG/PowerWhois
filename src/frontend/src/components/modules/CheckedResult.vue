<template>
    <q-table
        :rows="domainResults"
        row-key="domainID"
        :columns="tableColumns"
        :visibleColumns="visibleColumns"
        :selection="tokenStore.token ? 'multiple' : 'none'"
        v-model:selected="selectedDomains"
        virtual-scroll
        :virtual-scroll-sticky-start="true"
        :virtual-scroll-item-size="48"
        :virtual-scroll-slice-size="30"
        :rows-per-page-options="[0]"
        class="no-shadow bg-grey-1 checked-result-table"
        v-if="domainResults.length > 0"
    >
        <template v-slot:top>
            <q-btn
                size="sm"
                color="primary"
                class="q-mb-sm q-mr-sm"
                label="重查错误域名"
                @click="requerycheckedErrorDomains()"
                v-if="webStore.unCheckDomains < 1 && webStore.checkedErrorDomains > 0"
            ></q-btn>

            <q-btn
                size="sm"
                color="green"
                class="q-mb-sm"
                label="注册选中域名"
                :disable="webStore.unRegisterDomains > 0"
                @click="registerSelectedDomains()"
                v-if="tokenStore.token && selectedDomains.length > 0 && webStore.unCheckDomains < 1"
            ></q-btn>

            <q-space />

            <div class="text-caption q-gutter-xs">
                <span>
                    总共: <q-badge color="primary">{{ domainResults.length }}</q-badge>
                </span>
                <span>
                    已注册: <q-badge color="warning">{{ webStore.checkedTakenDomains }}</q-badge>
                </span>
                <span>
                    未注册: <q-badge color="secondary">{{ webStore.checkedFreeDomains }}</q-badge>
                </span>
                <span>
                    错误: <q-badge color="negative">{{ webStore.checkedErrorDomains }}</q-badge>
                </span>
                <span>
                    未处理: <q-badge color="accent">{{ webStore.unCheckDomains }}</q-badge>
                </span>
            </div>
        </template>

        <template v-slot:header-selection="scope">
            <q-checkbox v-model="scope.selected" />
        </template>

        <template v-slot:body-selection="scope">
            <q-checkbox v-model="scope.selected" />
        </template>

        <template v-slot:body-cell-typoType="props">
            <q-td :props="props">
                <div>
                    <span v-if="props.row.typoType in typoTypeCcTld">
                        {{ typoTypeCcTld[props.row.typoType] }}
                    </span>
                    <span v-else-if="props.row.typoType in typoTypes">{{ typoTypes[props.row.typoType] }}</span>
                    <span v-else>{{ props.row.typoType }}</span>
                </div>
            </q-td>
        </template>

        <template v-slot:body-cell-status="props">
            <q-td :props="props">
                <div>
                    <span class="cursor-pointer" v-if="props.row.isChecked" @click="showCheckedRawResponse(props.row)">
                        <q-chip dense color="warning" text-color="white" v-if="props.row.status == 'Taken'">{{ props.row.status }}</q-chip>
                        <q-chip dense color="secondary" text-color="white" v-if="props.row.status == 'Free'">{{ props.row.status }}</q-chip>
                        <q-chip dense color="negative" text-color="white" v-if="props.row.status == 'Error'">
                            {{ props.row.status }}
                            <q-tooltip class="bg-red" transition-show="scale" transition-hide="scale">
                                {{ props.row.errorInfo }}
                            </q-tooltip>
                        </q-chip>
                    </span>
                    <span v-else><q-spinner-ios color="primary" size="1.8em" /></span>
                </div>
            </q-td>
        </template>

        <template v-slot:body-cell-createdDate="props">
            <q-td :props="props">
                <div v-if="props.row.isChecked">
                    <span>{{ props.row.createdDate }}</span>
                </div>
            </q-td>
        </template>

        <template v-slot:body-cell-expiryDate="props">
            <q-td :props="props">
                <div v-if="props.row.isChecked">
                    <span>{{ props.row.expiryDate }}</span>
                </div>
            </q-td>
        </template>

        <template v-slot:body-cell-nameServer="props">
            <q-td :props="props">
                <div v-if="props.row.isChecked">
                    <div v-for="(dns_item, index) in props.row.nameServer" :key="index">
                        {{ dns_item }}
                    </div>
                </div>
            </q-td>
        </template>

        <template v-slot:body-cell-domainStatus="props">
            <q-td :props="props">
                <div v-if="props.row.isChecked">
                    <span>{{ props.row.domainStatus }}</span>
                </div>
            </q-td>
        </template>

        <template v-slot:body-cell-registerStatus="props">
            <q-td :props="props">
                <div>
                    <span
                        class="cursor-pointer"
                        v-if="props.row.selectedRegister && props.row.registerStatus"
                        @click="showRegisterRawResponse(props.row)"
                    >
                        <q-chip dense color="green" text-color="white" v-if="props.row.registerStatus == 'success'"> OK </q-chip>
                        <q-chip dense color="secondary" text-color="white" v-if="props.row.registerStatus == 'failed'"> Failed </q-chip>
                        <q-chip dense color="negative" text-color="white" v-if="props.row.registerStatus == 'error'"> Error </q-chip>
                    </span>
                    <span v-else-if="props.row.selectedRegister"><q-spinner-ios color="primary" size="1.8em" /></span>
                    <span v-else></span>
                </div>
            </q-td>
        </template>
    </q-table>

    <q-dialog v-model="showCheckedRawResponseDialog">
        <q-card style="min-width: 50vw; max-width: 100vw; max-height: 90vh">
            <q-card-section class="row items-center">
                <div class="text-h6">域名 {{ checkedRawResponseDomain }} 的原始查询响应</div>
                <q-space />
                <q-btn icon="close" flat round dense v-close-popup />
            </q-card-section>

            <q-separator />

            <q-card-section>
                <pre class="raw-response-content">{{ checkedRawResponseContent ? checkedRawResponseContent : "无响应内容" }}</pre>
            </q-card-section>
        </q-card>
    </q-dialog>

    <q-dialog v-model="showRegisterRawResponseDialog">
        <q-card style="min-width: 50vw; max-width: 100vw; max-height: 90vh">
            <q-card-section class="row items-center">
                <div class="text-h6">域名 {{ registerRawResponseDomain }} 的注册响应</div>
                <q-space />
                <q-btn icon="close" flat round dense v-close-popup />
            </q-card-section>

            <q-separator />

            <q-card-section>
                <pre class="raw-response-content">{{ registerRawResponseContent ? registerRawResponseContent : "无响应内容" }}</pre>
            </q-card-section>
        </q-card>
    </q-dialog>
</template>

<script setup>
import { ref, computed, onMounted, watch, watchEffect } from "vue";
import { useQuasar } from "quasar";
import { status, send } from "src/utils/websocketHandler";
import { useTokenStore } from "src/stores/tokenStore";
import { useWebStore } from "src/stores/webStore";
import { useSettingStore } from "src/stores/settingStore";
import { typoTypes, typoTypeCcTld } from "src/utils/globalDefinition";

defineOptions({
    name: "CheckedResult"
});

const props = defineProps({
    domainResults: {
        type: Array,
        default: () => []
    },
    resultType: {
        type: String,
        required: true
    },
    queryType: {
        type: String,
        required: true
    },
    registerType: {
        type: String,
        default: null
    }
});

const $q = useQuasar();

const tokenStore = useTokenStore();
const webStore = useWebStore();
const settingStore = useSettingStore();

const selectedDomains = ref([]);

const showCheckedRawResponseDialog = ref(false);
const checkedRawResponseDomain = ref(null);
const checkedRawResponseContent = ref(null);

const showRegisterRawResponseDialog = ref(false);
const registerRawResponseDomain = ref(null);
const registerRawResponseContent = ref(null);

const tableColumns = ref([
    { name: "domainID", label: "ID", field: "domainID", align: "center", sortable: true },
    { name: "domain", label: "域名", field: "domain", align: "left", sortable: true },
    { name: "typoType", label: "Typo类型", field: "typoType", align: "center", sortable: true },
    { name: "status", label: "状态", field: "status", align: "center", sortable: true },
    { name: "lookupType", label: "查询类型", field: "lookupType", align: "center", sortable: true },
    { name: "createdDate", label: "创建日期", field: "createdDate", align: "left", sortable: true },
    { name: "expiryDate", label: "过期日期", field: "expiryDate", align: "left", sortable: true },
    { name: "nameServer", label: "DNS", field: "nameServer", align: "left" },
    { name: "domainStatus", label: "域名状态", field: "domainStatus", align: "left", sortable: true },
    { name: "registerStatus", label: "注册状态", field: "registerStatus", align: "center", sortable: true },
    { name: "isChecked", label: "", field: "isChecked", align: "center" }
]);

const visibleColumns = ref(["domain", "status", "createdDate", "expiryDate", "nameServer", "domainStatus"]);


function updateVisibleColumns() {
    if (props.queryType == "whoisQuery" || props.queryType == "whoisQueryWithProxy") {
        if (props.resultType == "webCheck") {
            visibleColumns.value = ["domain", "status", "createdDate", "expiryDate", "nameServer", "domainStatus"];
        } else if (props.resultType == "typoCheck") {
            visibleColumns.value = ["domain", "typoType", "status", "createdDate", "expiryDate", "nameServer", "domainStatus"];
        }
    } else if (props.queryType == "dnsQuery") {
        if (props.resultType == "webCheck") {
            visibleColumns.value = ["domain", "status", "nameServer"];
        } else if (props.resultType == "typoCheck") {
            visibleColumns.value = ["domain", "status", "typoType", "nameServer"];
        }
    } else if (props.queryType == "mixedQuery") {
        if (props.resultType == "webCheck") {
            visibleColumns.value = ["domain", "status", "lookupType", "createdDate", "expiryDate", "nameServer", "domainStatus"];
        } else if (props.resultType == "typoCheck") {
            visibleColumns.value = ["domain", "typoType", "status", "lookupType", "createdDate", "expiryDate", "nameServer", "domainStatus"];
        }
    } else {
        if (props.resultType == "webCheck") {
            visibleColumns.value = ["domain", "status", "lookupType"];
        } else if (props.resultType == "typoCheck") {
            visibleColumns.value = ["domain", "typoType", "status", "lookupType"];
        }
    }

    if (tokenStore.token) {
        visibleColumns.value.push("registerStatus");
    }
}

function isWebsocketConnected() {
    if (status.value == "OPEN") {
        return true;
    } else {
        $q.notify({
            position: "top",
            type: "warning",
            message: "网络连接失败, 请尝试刷新网页后重试",
            progress: true,
            timeout: 5000,
            actions: [{ label: "确定", color: "white" }]
        });
        return false;
    }
}

function showCheckedRawResponse(data) {
    checkedRawResponseDomain.value = data.domain;
    checkedRawResponseContent.value = data.checkedRawResponse;
    showCheckedRawResponseDialog.value = true;
}

function showRegisterRawResponse(data) {
    registerRawResponseDomain.value = data.domain;
    registerRawResponseContent.value = data.registerRawResponse;
    showRegisterRawResponseDialog.value = true;
}

function requerycheckedErrorDomains() {
    if (isWebsocketConnected()) {
        webStore.setIsErrorRecheck(true);

        let errorDomainList = [];
        props.domainResults.forEach((domain) => {
            if (domain.status == "Error") {
                errorDomainList.push(domain.domain);
            }
        });

        if (errorDomainList.length > 0) {
            const webCheckData = {
                event: "webCheck",
                data: {
                    queryType: props.queryType,
                    domains: errorDomainList
                }
            };
            send(JSON.stringify(webCheckData));
        }
    }
}

function registerSelectedDomains() {
    if (selectedDomains.value.length == 0) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请先选择需要注册的域名"
        });
        return;
    }

    if (props.registerType == null) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请先选择注册接口"
        });
        return;
    }

    if (isWebsocketConnected()) {
        let registerDomainList = [];
        selectedDomains.value.forEach((domain) => {
            registerDomainList.push(domain.domain);
        });

        $q.dialog({
            title: `是否确认要注册选中的 ${registerDomainList.length} 个域名?`,
            cancel: "取消",
            ok: "确定",
            persistent: true
        }).onOk(() => {
            const registerData = {
                event: "register",
                data: {
                    registerType: props.registerType,
                    domains: registerDomainList
                }
            };
            send(JSON.stringify(registerData));

            webStore.setRegisteringDomains(selectedDomains.value);
        });
    }
}

onMounted(() => {
    updateVisibleColumns();
});

// watch(
//     () => webStore.checkedFreeDomains,
//     (newVal) => {
//         if (newVal > 0) {
//             webStore.domains.forEach((domain) => {
//                 if (domain.status == "Free") {
//                     selectedDomains.value.forEach((selectedDomain) => {
//                         if (selectedDomain.domain == domain.domain) {
//                             // 如果已经选中，则不添加
//                             return;
//                         }
//                     });
//                     selectedDomains.value.push(domain);
//                 }
//             });
//         } else {
//             selectedDomains.value = [];
//         }
//     }
// );
</script>

<style scoped>
.raw-response-content {
    white-space: pre-wrap;
    word-wrap: break-word;
    font-family: monospace;
    margin: 0;
    padding: 8px;
    background-color: #f5f5f5;
    border-radius: 4px;
    overflow-y: auto;
}

.checked-result-table {
    height: calc(100vh - 220px);
    max-height: calc(100vh - 220px);
}
</style>
