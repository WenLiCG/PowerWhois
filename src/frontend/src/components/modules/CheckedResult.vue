<template>
    <q-card flat class="workspace-card result-card" v-if="domainResults.length > 0">
        <div class="result-title"><h2>查询结果</h2><span>共 {{ domainResults.length }} 个域名</span></div>
        <q-table
            :rows="filteredResults"
            row-key="domainID"
            :columns="tableColumns"
            :visibleColumns="$q.screen.lt.sm ? visibleColumns.filter(c => ['domain', 'status', 'registerStatus'].includes(c)) : visibleColumns"
            v-model:pagination="tablePagination"
            :selection="canRegister ? 'multiple' : 'none'"
            v-model:selected="selectedDomains"
            :rows-per-page-options="[25, 50, 100]"
            flat
            aria-label="域名查询结果"
            no-data-label="没有匹配的域名，可更换状态或清空搜索。"
            class="bg-white"
        >
            <template v-slot:top>
                <div class="full-width">
                    <div class="row items-center q-gutter-sm">
                        <q-input dense outlined v-model="filter" aria-label="搜索结果域名" placeholder="搜索域名…" clearable class="result-search">
                            <template #prepend><q-icon name="filter_list" size="18px" /></template>
                        </q-input>
                        <q-btn
                            outline
                            no-caps
                            size="sm"
                            color="primary"
                            icon="refresh"
                            label="重查错误、超时和未知"
                            @click="requerycheckedErrorDomains()"
                            v-if="retryableCount > 0"
                            :disable="webStore.busy"
                            :loading="webStore.pendingQuery === 'retry'"
                        />
                        <q-btn
                            unelevated
                            no-caps
                            size="sm"
                            color="positive"
                            icon="shopping_cart_checkout"
                            :label="`注册选中 (${selectedDomains.length})`"
                            :disable="webStore.busy"
                            @click="registerSelectedDomains()"
                            v-if="canRegister && selectedDomains.length > 0 && webStore.unCheckDomains < 1"
                        />

                        <q-space />

                        <q-btn
                            flat
                            dense
                            no-caps
                            size="sm"
                            color="primary"
                            icon="content_copy"
                            label="复制域名"
                            :disable="filteredResults.length === 0"
                            @click="copyFilteredDomains"
                        >
                            <q-tooltip>复制当前筛选条件下的全部域名</q-tooltip>
                        </q-btn>
                        <q-btn
                            outline
                            dense
                            no-caps
                            size="sm"
                            color="primary"
                            icon="download"
                            label="导出 CSV"
                            :disable="filteredResults.length === 0"
                            class="q-px-sm"
                            @click="exportFilteredCsv"
                        >
                            <q-tooltip>导出当前筛选结果为 CSV 文件</q-tooltip>
                        </q-btn>
                    </div>

                    <div class="status-filters" role="group" aria-label="筛选查询状态">
                        <button v-for="item in filters" :key="item.key" type="button" :aria-pressed="statusFilter === item.key"
                            @click="toggleStatusFilter(item.key)">
                            {{ item.label }} <span>{{ item.count }}</span>
                        </button>
                        <span v-if="summaryCounts.pending" class="pending-count">待完成 {{ summaryCounts.pending }}</span>
                    </div>
                </div>
            </template>

            <template v-slot:header-selection="scope">
                <q-checkbox v-model="scope.selected" aria-label="全选域名" />
            </template>

            <template v-slot:body-selection="scope">
                <q-checkbox v-model="scope.selected" aria-label="选择域名" />
            </template>

            <template v-slot:body-cell-domain="props">
                <q-td :props="props">
                    <div class="row items-center no-wrap q-gutter-xs">
                        <q-btn
                            v-if="props.row.isChecked"
                            flat
                            dense
                            no-caps
                            size="sm"
                            color="primary"
                            class="domain-mono q-px-xs"
                            :label="props.row.domain"
                            @click="showCheckedRawResponse(props.row)"
                        />
                        <span v-else class="domain-mono">{{ props.row.domain }}</span>
                        <q-badge v-if="props.row.isChecked && props.row.viaProxy" outline color="indigo-7" class="q-px-xs">代理</q-badge>
                    </div>
                </q-td>
            </template>

            <template v-slot:body-cell-typoType="props">
                <q-td :props="props">
                    <span>{{ formatTypoTypes(props.row) }}</span>
                </q-td>
            </template>

            <template v-slot:body-cell-status="props">
                <q-td :props="props">
                    <div>
                        <q-btn
                            v-if="props.row.isChecked"
                            flat
                            dense
                            no-caps
                            icon-right="description"
                            size="sm"
                            :label="statusLabels[props.row.status] || props.row.status"
                            :color="statusColors[props.row.status] || 'grey-8'"
                            :aria-label="`${props.row.domain}：${statusLabels[props.row.status]}，查看原始响应`"
                            @click="showCheckedRawResponse(props.row)"
                        >
                            <q-tooltip>{{ props.row.errorInfo ? formatQueryError(props.row.errorInfo) : "点击查看来源与原始响应" }}</q-tooltip>
                        </q-btn>
                        <span v-else><q-spinner-ios color="primary" size="1.6em" /></span>
                    </div>
                </q-td>
            </template>

            <template v-slot:body-cell-createdDate="props">
                <q-td :props="props">
                    <span v-if="props.row.isChecked">{{ props.row.createdDate || "—" }}</span>
                </q-td>
            </template>

            <template v-slot:body-cell-expiryDate="props">
                <q-td :props="props">
                    <span v-if="props.row.isChecked">{{ props.row.expiryDate || "—" }}</span>
                </q-td>
            </template>

            <template v-slot:body-cell-nameServer="props">
                <q-td :props="props">
                    <div v-if="props.row.isChecked">
                        <div v-for="(dnsItem, index) in props.row.nameServer" :key="index">
                            {{ dnsItem }}
                        </div>
                    </div>
                </q-td>
            </template>

            <template v-slot:body-cell-domainStatus="props">
                <q-td :props="props">
                    <span v-if="props.row.isChecked">{{ props.row.domainStatus || "—" }}</span>
                </q-td>
            </template>

            <template v-slot:body-cell-registerStatus="props">
                <q-td :props="props">
                    <div>
                        <q-btn
                            v-if="props.row.selectedRegister && props.row.registerStatus"
                            flat
                            dense
                            no-caps
                            size="sm"
                            icon-right="receipt_long"
                            :label="registrationLabels[props.row.registerStatus] || props.row.registerStatus"
                            :color="props.row.registerStatus === 'success' ? 'positive' : props.row.registerStatus === 'outcome_unknown' ? 'warning' : 'negative'"
                            :aria-label="`${props.row.domain}，${registrationLabels[props.row.registerStatus]}，查看注册详情`"
                            @click="showRegisterRawResponse(props.row)"
                        />
                        <span v-else-if="props.row.selectedRegister"><q-spinner-ios color="primary" size="1.6em" /></span>
                    </div>
                </q-td>
            </template>
        </q-table>
    </q-card>

    <q-dialog v-model="showCheckedRawResponseDialog">
        <q-card style="width: 760px; max-width: 94vw; max-height: 90vh">
            <q-card-section class="row items-center q-py-sm">
                <div class="text-subtitle1 text-weight-bold domain-mono">{{ checkedDetailRow?.domain }}</div>
                <q-badge v-if="checkedDetailRow?.status" :color="statusColors[checkedDetailRow.status] || 'grey-8'" class="q-ml-sm">
                    {{ statusLabels[checkedDetailRow.status] || checkedDetailRow.status }}
                </q-badge>
                <q-badge v-if="checkedDetailRow?.viaProxy" outline color="indigo-7" class="q-ml-xs">经由 SOCKS5 代理</q-badge>
                <q-space />
                <q-btn
                    flat
                    dense
                    no-caps
                    icon="content_copy"
                    label="复制原文"
                    color="primary"
                    class="q-mr-xs"
                    :disable="!checkedDetailRow?.rawResponse"
                    @click="copyText(checkedDetailRow.rawResponse, '原始响应已复制')"
                />
                <q-btn icon="close" aria-label="关闭详情" flat round dense v-close-popup />
            </q-card-section>

            <q-separator />

            <q-card-section>
                <div v-if="checkedDetailRow" class="detail-meta text-body2 q-mb-sm">
                    <div v-if="formatTypoTypes(checkedDetailRow)"><strong>变体规则：</strong>{{ formatTypoTypes(checkedDetailRow) }}</div>
                    <div v-if="checkedDetailRow.source || checkedDetailRow.lookupType">
                        <strong>查询来源：</strong>{{ checkedDetailRow.source || "—" }}（{{ checkedDetailRow.lookupType || "—" }}）
                    </div>
                    <div v-if="checkedDetailRow.queriedAt">
                        <strong>查询时间：</strong>{{ formatDateTime(checkedDetailRow.queriedAt) }}
                    </div>
                    <div v-if="checkedDetailRow.createdDate || checkedDetailRow.expiryDate">
                        <strong>创建 / 到期：</strong>{{ checkedDetailRow.createdDate || "—" }} → {{ checkedDetailRow.expiryDate || "—" }}
                    </div>
                    <div v-if="checkedDetailRow.domainStatus">
                        <strong>域名状态：</strong>{{ checkedDetailRow.domainStatus }}
                    </div>
                    <div v-if="checkedDetailRow.nameServer?.length && checkedDetailRow.nameServer[0] !== 'N/A'">
                        <strong>权威 DNS：</strong>{{ checkedDetailRow.nameServer.join(", ") }}
                    </div>
                </div>
                <q-banner v-if="checkedDetailRow?.errorInfo" dense class="bg-orange-1 text-deep-orange-10 q-mb-sm rounded-borders">
                    <div>{{ formatQueryError(checkedDetailRow.errorInfo) }}</div>
                    <div v-if="formatQueryError(checkedDetailRow.errorInfo) !== checkedDetailRow.errorInfo" class="text-caption text-grey-8 q-mt-xs">
                        原始信息：{{ checkedDetailRow.errorInfo }}
                    </div>
                </q-banner>
                <pre class="raw-response-box">{{ checkedDetailRow?.rawResponse || "无原始响应" }}</pre>
            </q-card-section>
        </q-card>
    </q-dialog>

    <q-dialog v-model="showRegisterRawResponseDialog">
        <q-card style="width: 760px; max-width: 94vw; max-height: 90vh">
            <q-card-section class="row items-center q-py-sm">
                <div class="text-subtitle1 text-weight-bold">{{ registerRawResponseDomain }} · 注册响应详情</div>
                <q-space />
                <q-btn
                    flat
                    dense
                    no-caps
                    icon="content_copy"
                    label="复制内容"
                    color="primary"
                    class="q-mr-xs"
                    :disable="!registerRawResponseContent"
                    @click="copyText(registerRawResponseContent, '注册响应已复制')"
                />
                <q-btn icon="close" aria-label="关闭详情" flat round dense v-close-popup />
            </q-card-section>

            <q-separator />

            <q-card-section>
                <pre class="raw-response-box">{{ registerRawResponseContent ? registerRawResponseContent : "无响应内容" }}</pre>
            </q-card-section>
        </q-card>
    </q-dialog>
</template>

<script setup>
import { ref, computed, onMounted, watch } from "vue";
import { useQuasar, copyToClipboard, date } from "quasar";
import { status, send, submitQuery } from "src/utils/websocketHandler";
import { useTokenStore } from "src/stores/tokenStore";
import { useWebStore } from "src/stores/webStore";
import { useSettingStore } from "src/stores/settingStore";
import { typoTypes, typoTypeCcTld } from "src/utils/globalDefinition";
import { buildWebResultsCsv, downloadCsvContent } from "src/utils/exportCsv";
import { formatQueryError, formatDateTime } from "src/utils/formatError";

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
    outcome_unknown: "待核实订单"
};

const showCheckedRawResponseDialog = ref(false);
const checkedDetailRow = ref(null);

const showRegisterRawResponseDialog = ref(false);
const registerRawResponseDomain = ref(null);
const registerRawResponseContent = ref(null);

const tableColumns = ref([
    { name: "domainID", label: "ID", field: "domainID", align: "center", sortable: true },
    { name: "domain", label: "域名", field: "domain", align: "left", sortable: true },
    { name: "typoType", label: "Typo 类型", field: "typoType", align: "left", sortable: true },
    { name: "status", label: "状态", field: "status", align: "center", sortable: true },
    { name: "lookupType", label: "协议", field: "lookupType", align: "center", sortable: true },
    { name: "createdDate", label: "创建日期", field: "createdDate", align: "left", sortable: true },
    { name: "expiryDate", label: "过期日期", field: "expiryDate", align: "left", sortable: true },
    { name: "nameServer", label: "权威 DNS", field: "nameServer", align: "left" },
    { name: "domainStatus", label: "域名状态", field: "domainStatus", align: "left", sortable: true },
    { name: "registerStatus", label: "注册结果", field: "registerStatus", align: "center", sortable: true },
    { name: "isChecked", label: "", field: "isChecked", align: "center" }
]);

const visibleColumns = ref(["domain", "status", "createdDate", "expiryDate", "nameServer", "domainStatus"]);

const filter = ref("");
const statusFilter = ref("all");
const statusLabels = { Taken: "已注册", NotFound: "未找到", Free: "未找到", Unknown: "未知", Timeout: "超时", Error: "错误" };
const statusColors = { Taken: "primary", NotFound: "teal-9", Free: "teal-9", Unknown: "blue-grey-8", Timeout: "deep-orange-9", Error: "negative" };

const summaryCounts = computed(() => {
    const counts = { taken: 0, free: 0, unknown: 0, timeout: 0, error: 0, pending: 0 };
    for (const row of props.domainResults) {
        if (!row.isChecked) counts.pending++;
        else if (row.status === "Taken") counts.taken++;
        else if (row.status === "NotFound" || row.status === "Free") counts.free++;
        else if (row.status === "Unknown") counts.unknown++;
        else if (row.status === "Timeout") counts.timeout++;
        else if (row.status === "Error") counts.error++;
    }
    return counts;
});

const retryableCount = computed(() => summaryCounts.value.error + summaryCounts.value.timeout + summaryCounts.value.unknown);
const filters = computed(() => [
    { key: "all", label: "全部", count: props.domainResults.length },
    ...[["Taken", "taken"], ["NotFound", "free"], ["Unknown", "unknown"], ["Timeout", "timeout"], ["Error", "error"]]
        .map(([key, count]) => ({ key, label: statusLabels[key], count: summaryCounts.value[count] }))
]);

const filteredResults = computed(() => {
    const keyword = (filter.value || "").trim().toLowerCase();
    return props.domainResults.filter((d) => {
        if (keyword && !d.domain.includes(keyword)) return false;
        if (statusFilter.value === "all") return true;
        if (statusFilter.value === "NotFound") return d.status === "NotFound" || d.status === "Free";
        return d.status === statusFilter.value;
    });
});

const tablePagination = ref({ rowsPerPage: 25 });

function toggleStatusFilter(next) {
    statusFilter.value = statusFilter.value === next ? "all" : next;
}

function formatTypoTypes(row) {
    const list = Array.isArray(row.typoTypes) && row.typoTypes.length ? row.typoTypes : row.typoType ? [row.typoType] : [];
    return list.map((t) => typoTypeCcTld[t] || typoTypes[t] || t).join(" / ");
}

async function copyText(text, successMsg) {
    try {
        await copyToClipboard(text);
        $q.notify({ position: "top", type: "positive", message: successMsg });
    } catch {
        $q.notify({ position: "top", type: "warning", message: "复制失败，请手动选中复制" });
    }
}

async function copyFilteredDomains() {
    const names = filteredResults.value.map((r) => r.domain);
    if (!names.length) return;
    await copyText(names.join("\n"), `已复制 ${names.length} 个域名`);
}

function exportFilteredCsv() {
    if (!filteredResults.value.length) return;
    const csv = buildWebResultsCsv(filteredResults.value, props.resultType === "typoCheck");
    const stamp = date.formatDate(Date.now(), "YYYYMMDD-HHmmss");
    downloadCsvContent(csv, `powerwhois-${props.resultType}-${stamp}.csv`);
}

// Columns follow the modes that produced these rows, not the selector the user may have changed since.
const resultMode = computed(() => {
    const modes = webStore.resultModes[props.resultType] || [];
    if (modes.length === 1) return modes[0];
    return modes.length > 1 ? "mixedQuery" : props.queryType;
});

function updateVisibleColumns() {
    const mode = resultMode.value;
    if (mode == "whoisQuery" || mode == "whoisQueryWithProxy") {
        if (props.resultType == "webCheck") {
            visibleColumns.value = ["domain", "status", "createdDate", "expiryDate", "nameServer", "domainStatus"];
        } else if (props.resultType == "typoCheck") {
            visibleColumns.value = ["domain", "typoType", "status", "createdDate", "expiryDate", "nameServer", "domainStatus"];
        }
    } else if (mode == "dnsQuery") {
        if (props.resultType == "webCheck") {
            visibleColumns.value = ["domain", "status", "nameServer"];
        } else if (props.resultType == "typoCheck") {
            visibleColumns.value = ["domain", "typoType", "status", "nameServer"];
        }
    } else if (mode == "mixedQuery") {
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

    if (canRegister.value || (tokenStore.token && props.domainResults.some((row) => row.selectedRegister))) {
        visibleColumns.value.push("registerStatus");
    }
}

function isWebsocketConnected() {
    if (status.value == "OPEN") {
        return true;
    }
    $q.notify({
        position: "top",
        type: "warning",
        message: "网络连接已断开，请点击右上角“重连”或刷新页面后重试",
        progress: true,
        timeout: 5000,
        actions: [{ label: "确定", color: "white" }]
    });
    return false;
}

function showCheckedRawResponse(data) {
    checkedDetailRow.value = data;
    showCheckedRawResponseDialog.value = true;
}

function showRegisterRawResponse(data) {
    registerRawResponseDomain.value = data.domain;
    registerRawResponseContent.value = data.registerRawResponse;
    showRegisterRawResponseDialog.value = true;
}

function requerycheckedErrorDomains() {
    const domains = props.domainResults.filter((row) => ["Error", "Timeout", "Unknown"].includes(row.status)).map((row) => row.domain);
    // The retry uses the currently selected mode (e.g. switch to the proxy) and replaces rows of this tab only.
    if (domains.length) submitQuery("webCheck", { queryType: props.queryType, domains }, props.resultType);
}

function registerSelectedDomains() {
    if (selectedDomains.value.length == 0) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请先勾选需要注册的域名"
        });
        return;
    }

    if (props.registerType == null) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请先展开上方的「手动注册接口」并选择注册商"
        });
        return;
    }

    if (isWebsocketConnected()) {
        const rows = [...selectedDomains.value];
        const registerDomainList = [...new Set(rows.map((row) => row.domain))];
        const registrar = props.registerType;
        const resultRows = webStore.domains;
        const token = tokenStore.token;

        $q.dialog({
            title: `是否确认要注册选中的 ${registerDomainList.length} 个域名？`,
            message: `注册接口：${registrar}。域名：${registerDomainList.slice(0, 5).join("、")}${rows.length > 5 ? " 等" : ""}。提交可能扣费且无法撤回，断线后请先向注册商核实订单。`,
            cancel: "取消",
            ok: "确认提交注册",
            persistent: true
        }).onOk(() => {
            if (!token || tokenStore.token !== token || webStore.busy || webStore.domains !== resultRows || !settingStore.registerApis.includes(registrar)) {
                $q.notify({ type: "warning", message: "会话、设置或结果已变更，请重新确认注册。" });
                return;
            }
            const registerData = {
                event: "register",
                data: {
                    registerType: registrar,
                    confirmed: true,
                    domains: registerDomainList
                }
            };
            if (send(JSON.stringify(registerData), false)) {
                webStore.setRegisteringDomains(rows);
            } else {
                $q.notify({ type: "warning", message: "连接已中断，注册请求未发送。" });
            }
        });
    }
}

onMounted(() => {
    updateVisibleColumns();
});

watch(() => [resultMode.value, props.resultType, canRegister.value], updateVisibleColumns);
watch(
    () => props.domainResults,
    () => {
        selectedDomains.value = [];
    }
);
</script>
