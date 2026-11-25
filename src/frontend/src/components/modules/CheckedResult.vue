<template>
    <q-card flat class="workspace-card overflow-hidden" v-if="domainResults.length > 0">
        <q-table
            :rows="filteredResults"
            row-key="domainID"
            :columns="tableColumns"
            :visibleColumns="visibleColumns"
            v-model:pagination="tablePagination"
            :selection="canRegister ? 'multiple' : 'none'"
            v-model:selected="selectedDomains"
            :rows-per-page-options="[25, 50, 100]"
            flat
            class="bg-white"
        >
            <template v-slot:top>
                <div class="full-width">
                    <div class="row items-center q-gutter-sm">
                        <q-input dense outlined v-model="filter" placeholder="搜索域名…" clearable style="min-width: 190px">
                            <template #prepend><q-icon name="filter_list" size="18px" /></template>
                        </q-input>
                        <q-select
                            dense
                            outlined
                            v-model="statusFilter"
                            :options="statusOptions"
                            emit-value
                            map-options
                            label="状态筛选"
                            class="status-filter"
                        />
                        <q-btn
                            outline
                            no-caps
                            size="sm"
                            color="primary"
                            icon="refresh"
                            label="重查错误、超时和未知"
                            @click="requerycheckedErrorDomains()"
                            v-if="webStore.unCheckDomains < 1 && webStore.unRegisterDomains < 1 && retryableCount > 0"
                        />
                        <q-btn
                            unelevated
                            no-caps
                            size="sm"
                            color="positive"
                            icon="shopping_cart_checkout"
                            :label="`注册选中 (${selectedDomains.length})`"
                            :disable="webStore.unRegisterDomains > 0"
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

                    <div class="row items-center q-gutter-xs q-mt-sm text-caption">
                        <q-chip
                            clickable
                            dense
                            square
                            :outline="statusFilter !== 'all'"
                            color="primary"
                            text-color="white"
                            @click="toggleStatusFilter('all')"
                        >
                            全部 {{ domainResults.length }}
                        </q-chip>
                        <q-chip
                            clickable
                            dense
                            square
                            :outline="statusFilter !== 'Taken'"
                            color="blue-8"
                            text-color="white"
                            @click="toggleStatusFilter('Taken')"
                        >
                            已注册 {{ summaryCounts.taken }}
                        </q-chip>
                        <q-chip
                            clickable
                            dense
                            square
                            :outline="statusFilter !== 'NotFound'"
                            color="teal-8"
                            text-color="white"
                            @click="toggleStatusFilter('NotFound')"
                        >
                            未找到 {{ summaryCounts.free }}
                        </q-chip>
                        <q-chip
                            clickable
                            dense
                            square
                            :outline="statusFilter !== 'Unknown'"
                            color="blue-grey-7"
                            text-color="white"
                            @click="toggleStatusFilter('Unknown')"
                        >
                            未知 {{ summaryCounts.unknown }}
                        </q-chip>
                        <q-chip
                            clickable
                            dense
                            square
                            :outline="statusFilter !== 'Timeout'"
                            color="deep-orange-8"
                            text-color="white"
                            @click="toggleStatusFilter('Timeout')"
                        >
                            超时 {{ summaryCounts.timeout }}
                        </q-chip>
                        <q-chip
                            clickable
                            dense
                            square
                            :outline="statusFilter !== 'Error'"
                            color="negative"
                            text-color="white"
                            @click="toggleStatusFilter('Error')"
                        >
                            错误 {{ summaryCounts.error }}
                        </q-chip>
                        <q-chip v-if="summaryCounts.pending > 0" dense square outline color="accent">
                            待完成 {{ summaryCounts.pending }}
                        </q-chip>
                        <span class="text-grey-7 q-ml-xs">点击状态标签可筛选，点击行内状态可查看原始响应</span>
                    </div>
                </div>
            </template>

            <template v-slot:header-selection="scope">
                <q-checkbox v-model="scope.selected" aria-label="全选域名" />
            </template>

            <template v-slot:body-selection="scope">
                <q-checkbox v-model="scope.selected" aria-label="选择域名" />
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
                            <q-tooltip>{{ props.row.errorInfo ? props.row.errorInfo : "点击查看来源与原始响应" }}</q-tooltip>
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
                <div class="text-subtitle1 text-weight-bold">{{ checkedRawResponseDomain }} · 原始查询证据</div>
                <q-space />
                <q-btn
                    flat
                    dense
                    no-caps
                    icon="content_copy"
                    label="复制内容"
                    color="primary"
                    class="q-mr-xs"
                    :disable="!checkedRawResponseContent"
                    @click="copyText(checkedRawResponseContent, '原始响应已复制')"
                />
                <q-btn icon="close" aria-label="关闭详情" flat round dense v-close-popup />
            </q-card-section>

            <q-separator />

            <q-card-section>
                <pre class="raw-response-box">{{ checkedRawResponseContent ? checkedRawResponseContent : "无响应内容" }}</pre>
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
import { status, send } from "src/utils/websocketHandler";
import { useTokenStore } from "src/stores/tokenStore";
import { useWebStore } from "src/stores/webStore";
import { useSettingStore } from "src/stores/settingStore";
import { typoTypes, typoTypeCcTld } from "src/utils/globalDefinition";
import { buildWebResultsCsv, downloadCsvContent } from "src/utils/exportCsv";

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
const checkedRawResponseDomain = ref(null);
const checkedRawResponseContent = ref(null);

const showRegisterRawResponseDialog = ref(false);
const registerRawResponseDomain = ref(null);
const registerRawResponseContent = ref(null);

const tableColumns = ref([
    { name: "domainID", label: "ID", field: "domainID", align: "center", sortable: true },
    { name: "domain", label: "域名", field: "domain", align: "left", sortable: true },
    { name: "typoType", label: "Typo 类型", field: "typoType", align: "left", sortable: true },
    { name: "status", label: "状态（点击查看原文）", field: "status", align: "center", sortable: true },
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
const statusOptions = [
    { label: "全部状态", value: "all" },
    ...Object.entries(statusLabels)
        .filter(([key]) => key !== "Free")
        .map(([value, label]) => ({ value, label }))
];

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
            visibleColumns.value = ["domain", "typoType", "status", "nameServer"];
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
        const errorDomainList = [];
        props.domainResults.forEach((domain) => {
            if (["Error", "Timeout", "Unknown"].includes(domain.status)) {
                errorDomainList.push(domain.domain);
            }
        });

        if (errorDomainList.length > 0) {
            webStore.setIsErrorRecheck(true);
            send(
                JSON.stringify({
                    event: "webCheck",
                    data: {
                        queryType: props.queryType,
                        domains: errorDomainList
                    }
                })
            );
        }
    }
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
            message: "请先在上方选择注册接口"
        });
        return;
    }

    if (isWebsocketConnected()) {
        const registerDomainList = [...new Set(selectedDomains.value.map((domain) => domain.domain))];

        $q.dialog({
            title: `是否确认要注册选中的 ${registerDomainList.length} 个域名？`,
            message: `注册接口：${props.registerType}。提交可能产生注册商扣费，已发出的请求无法撤回。若中途断线，请先前往注册商后台核实订单。`,
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
watch(
    () => props.domainResults,
    () => {
        selectedDomains.value = [];
    }
);
</script>
