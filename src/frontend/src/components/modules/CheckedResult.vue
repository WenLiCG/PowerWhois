<template>
    <q-table
        :rows="domainResults"
        row-key="domainID"
        :columns="tableColumns"
        :visibleColumns="visibleColumns"
        :pagination="tablePagination"
        :selection="tokenStore.token ? 'multiple' : 'none'"
        v-model:selected="selectedDomains"
        hide-bottom
        class="no-shadow bg-grey-1"
        v-if="domainResults.length > 0"
    >
        <template v-slot:top>
            <q-btn
                size="sm"
                color="primary"
                class="q-mb-sm q-mr-sm"
                label="重查错误域名"
                @click="requerycheckedErrorDomains()"
                v-if="!webStore.checking && domainResults.some((row) => row.status === 'Error')"
            ></q-btn>

            <q-btn
                size="sm"
                color="green"
                class="q-mb-sm"
                label="注册选中域名"
                :disable="webStore.unRegisterDomains > 0"
                @click="registerSelectedDomains()"
                v-if="tokenStore.token && eligibleSelected.length > 0 && !webStore.checking"
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
                    未找到注册记录: <q-badge color="secondary">{{ webStore.checkedFreeDomains }}</q-badge>
                </span>
                <span>
                    证据不足或保留: <q-badge color="grey">{{ webStore.checkedUnknownDomains }}</q-badge>
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
                        <q-chip dense color="secondary" text-color="white" v-if="props.row.status == 'Free'">未找到
                            <q-tooltip>{{ props.row.errorInfo }}</q-tooltip>
                        </q-chip>
                        <q-chip dense color="grey" text-color="white" v-if="['Unknown', 'Reserved'].includes(props.row.status)">
                            {{ props.row.status === 'Reserved' ? '注册局保留' : '证据不足' }}
                            <q-tooltip>{{ props.row.errorInfo }}</q-tooltip>
                        </q-chip>
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
                        <q-chip dense color="orange" text-color="white" v-if="props.row.registerStatus == 'outcome_unknown'">待核对</q-chip>
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
                <div class="text-h6">域名 {{ registerRow?.domain }} 的注册响应</div>
                <q-space />
                <q-btn icon="close" flat round dense v-close-popup />
            </q-card-section>

            <q-separator />

            <q-card-section>
                <pre class="raw-response-content">{{ registerRow?.registerRawResponse || "无响应内容" }}</pre>
                <div v-if="registerRow?.operationId">操作编号 {{ registerRow.operationId }}</div>
            </q-card-section>
            <q-card-actions v-if="tokenStore.token && registerRow?.registerStatus === 'outcome_unknown'" align="right">
                <q-btn flat label="已核实失败" :disable="reconciling" @click="reconcile('failed')" />
                <q-btn flat color="primary" label="已核实成功" :disable="reconciling" @click="reconcile('success')" />
            </q-card-actions>
        </q-card>
    </q-dialog>
</template>

<script setup>
import { ref, computed, watch, watchEffect } from "vue";
import { api } from "boot/axios";
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
const eligibleSelected = computed(() => {
    const selected = new Set(selectedDomains.value);
    return props.domainResults.filter((row) => selected.has(row) && row.status === "Free" &&
        !["outcome_unknown", "success"].includes(row.registerStatus));
});

const showCheckedRawResponseDialog = ref(false);
const checkedRawResponseDomain = ref(null);
const checkedRawResponseContent = ref(null);

const showRegisterRawResponseDialog = ref(false);
const registerRow = ref(null);
const reconciling = ref(false);

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

const tablePagination = ref({ rowsPerPage: 10000 });

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
    registerRow.value = data;
    showRegisterRawResponseDialog.value = true;
}

function requerycheckedErrorDomains() {
    if (!isWebsocketConnected() || webStore.checking) return;
    const domains = props.domainResults.filter((row) => row.status === "Error").map((row) => row.domain);
    if (domains.length && send(JSON.stringify({ event: "webCheck", data: { queryType: props.queryType, domains } }))) {
        webStore.beginCheck(true);
    }
}

function registerSelectedDomains() {
    const domains = [...new Set(eligibleSelected.value.map((row) => row.domain))];
    const registerType = props.registerType;
    if (!registerType) {
        $q.notify({ position: "top", type: "warning", message: "请先选择注册接口" });
        return;
    }
    if (!domains.length || webStore.unRegisterDomains || !isWebsocketConnected()) return;
    if (!globalThis.crypto?.randomUUID) {
        $q.notify({ position: "top", type: "warning", message: "请在 HTTPS 页面提交注册" });
        return;
    }
    const operationId = globalThis.crypto.randomUUID();
    $q.dialog({
        title: "确认注册 " + domains.length + " 个域名",
        message: "通过 " + registerType + " 提交，价格和可注册性以注册商为准。域名清单 " + domains.join("、"),
        cancel: "取消",
        ok: "确认提交",
        persistent: true
    }).onOk(() => {
        if (webStore.unRegisterDomains || webStore.checking || domains.some((name) => webStore.byName[name]?.status !== "Free")) return;
        const data = { registerType, operationId, domains };
        if (send(JSON.stringify({ event: "register", data }))) webStore.setRegisteringDomains(domains, operationId);
    });
}

function reconcile(outcome) {
    const row = registerRow.value;
    if (!row?.operationId || reconciling.value) return;
    $q.dialog({
        title: "保存人工核对结果",
        message: "请先在注册商确认 " + row.domain + " 的订单结果。确认失败后可以重新提交注册。",
        cancel: true
    }).onOk(async () => {
        reconciling.value = true;
        try {
            const { data } = await api.put("/admin/registration/" + row.operationId, { domain: row.domain, outcome });
            webStore.updateRegisterResult(data);
        } catch {
            $q.notify({ position: "top", type: "negative", message: "找不到可更新的操作或仍在处理，请核对注册商" });
        } finally {
            reconciling.value = false;
        }
    });
}

watchEffect(updateVisibleColumns);
watch(() => props.domainResults, (rows) => {
    if (!rows.some((row) => selectedDomains.value.includes(row))) selectedDomains.value = [];
});
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
</style>
