<template>
    <q-table
        :rows="filteredResults"
        row-key="domainID"
        :columns="tableColumns"
        :visibleColumns="visibleColumns"
        v-model:pagination="tablePagination"
        :selection="canRegister ? 'multiple' : 'none'"
        v-model:selected="selectedDomains"
        :rows-per-page-options="[25, 50, 100]"
        class="no-shadow bg-grey-1"
        v-if="domainResults.length > 0"
    >
        <template v-slot:top>
            <q-input dense outlined v-model="filter" label="筛选域名" clearable class="q-mr-sm q-mb-sm" />
            <q-select dense outlined v-model="statusFilter" :options="statusOptions" emit-value map-options label="结果状态" class="q-mr-sm q-mb-sm status-filter" />
            <q-btn
                size="sm"
                color="primary"
                class="q-mb-sm q-mr-sm"
                label="重查错误、超时和未知"
                @click="requerycheckedErrorDomains()"
                v-if="webStore.unCheckDomains < 1 && webStore.unRegisterDomains < 1 && (webStore.checkedErrorDomains + webStore.checkedTimeoutDomains + webStore.checkedUnknownDomains) > 0"
            ></q-btn>

            <q-btn
                size="sm"
                color="green"
                class="q-mb-sm"
                label="注册选中域名"
                :disable="webStore.unRegisterDomains > 0"
                @click="registerSelectedDomains()"
                v-if="canRegister && selectedDomains.length > 0 && webStore.unCheckDomains < 1"
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
                    未找到: <q-badge color="secondary">{{ webStore.checkedFreeDomains }}</q-badge>
                </span>
                <span>
                    未知: <q-badge color="grey-8">{{ webStore.checkedUnknownDomains }}</q-badge> · 超时: <q-badge color="deep-orange-9">{{ webStore.checkedTimeoutDomains }}</q-badge> · 错误: <q-badge color="negative">{{ webStore.checkedErrorDomains }}</q-badge>
                </span>
                <span>
                    未处理: <q-badge color="accent">{{ webStore.unCheckDomains }}</q-badge>
                </span>
            </div>
        </template>

        <template v-slot:header-selection="scope">
            <q-checkbox v-model="scope.selected" aria-label="选择域名" />
        </template>

        <template v-slot:body-selection="scope">
            <q-checkbox v-model="scope.selected" aria-label="选择域名" />
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
                    <q-btn v-if="props.row.isChecked" flat dense no-caps :label="statusLabels[props.row.status] || props.row.status" :color="statusColors[props.row.status] || 'grey-8'" :aria-label="`${props.row.domain}：${statusLabels[props.row.status]}，查看原始响应`" @click="showCheckedRawResponse(props.row)">
                        <q-tooltip v-if="props.row.errorInfo">{{ props.row.errorInfo }}</q-tooltip>
                    </q-btn>
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
                    <q-btn
                        v-if="props.row.selectedRegister && props.row.registerStatus"
                        flat dense no-caps
                        :label="registrationLabels[props.row.registerStatus] || props.row.registerStatus"
                        :color="props.row.registerStatus === 'success' ? 'positive' : 'negative'"
                        :aria-label="`${props.row.domain}，${registrationLabels[props.row.registerStatus]}，查看注册详情`"
                        @click="showRegisterRawResponse(props.row)"
                    />
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
                <q-btn icon="close" aria-label="关闭详情" flat round dense v-close-popup />
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
                <q-btn icon="close" aria-label="关闭详情" flat round dense v-close-popup />
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

const canRegister = computed(() => !!tokenStore.token && settingStore.registerApis.length > 0);
const selectedDomains = ref([]);
const registrationLabels = {
    success: "注册成功",
    failed: "注册失败",
    error: "注册出错",
    outcome_unknown: "结果待核实，请先查注册商"
};

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

const filter = ref('');
const statusFilter = ref('all');
const statusLabels = { Taken: '已注册', NotFound: '未找到', Free: '未找到', Unknown: '未知', Timeout: '超时', Error: '错误' };
const statusColors = { Taken: 'primary', NotFound: 'teal-9', Unknown: 'grey-8', Timeout: 'deep-orange-9', Error: 'negative' };
const statusOptions = [{ label: '全部状态', value: 'all' }, ...Object.entries(statusLabels).filter(([key]) => key !== 'Free').map(([value,label]) => ({ value,label }))];
const filteredResults = computed(() => props.domainResults.filter(d => (!filter.value || d.domain.includes(filter.value.toLowerCase())) && (statusFilter.value === 'all' || d.status === statusFilter.value)));
const tablePagination = ref({ rowsPerPage: 25 });

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

    if (canRegister.value || (tokenStore.token && props.domainResults.some(row => row.selectedRegister))) {
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
            if (["Error", "Timeout", "Unknown"].includes(domain.status)) {
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
            message: `接口 ${props.registerType}。提交可能产生费用，已提交的请求无法撤回。若曾断线，请先向注册商核实订单。`,
            cancel: "取消",
            ok: "确认提交注册",
            persistent: true
        }).onOk(() => {
            const registerData = {
                event: "register",
                data: {
                    registerType: props.registerType,
                    confirmed: true,
                    domains: registerDomainList
                }
            };
            // A paid operation must never be buffered for automatic replay on reconnect.
            if (send(JSON.stringify(registerData), false)) {
                webStore.setRegisteringDomains(selectedDomains.value);
            } else {
                $q.notify({ type: "warning", message: "连接已中断，注册请求未发送。" });
            }
        });
    }
}

onMounted(() => {
    updateVisibleColumns();
});

watch(() => [props.queryType, props.resultType, canRegister.value], updateVisibleColumns);
watch(() => props.domainResults, () => { selectedDomains.value = []; });
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
