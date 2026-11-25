<template>
    <section class="batch-workspace">
        <div class="section-heading">
            <div>
                <h2>持久化批量任务</h2>
                <p>关闭浏览器后仍会在服务端继续执行。Web、CLI、HTTP API 与 MCP 共用同一任务历史。</p>
            </div>
            <q-btn outline no-caps color="primary" icon="refresh" label="刷新列表" :loading="refreshing" @click="refresh" />
        </div>
        <q-banner v-if="error" class="bg-red-1 text-negative q-mb-md rounded-borders" role="alert"
            >{{ error }}<template #action><q-btn flat no-caps label="关闭提示" @click="error = ''" /></template
        ></q-banner>
        <q-banner v-if="refreshError" class="bg-red-1 text-negative q-mb-md rounded-borders" role="alert"
            >{{ refreshError }}<template #action><q-btn flat no-caps label="重试刷新" @click="refresh" /></template
        ></q-banner>

        <q-card flat class="workspace-card q-mb-md">
            <q-expansion-item label="新建查询任务" icon="add_circle_outline" default-opened header-class="text-weight-bold text-grey-9">
                <q-separator />
                <q-form @submit.prevent="createTask" class="q-pa-md">
                    <q-file
                        outlined
                        v-model="file"
                        accept=".txt,text/plain"
                        label="选择域名 TXT 文件"
                        hint="每行一个纯域名，服务端自动规范化与去重；单批最多 100 万个域名，文件不超过 50 MB。"
                        clearable
                        :disable="creating"
                    >
                        <template #prepend><q-icon name="upload_file" /></template>
                    </q-file>
                    <WhoisSelection v-model:queryType="mode" :disable="creating" />
                    <div class="batch-options q-mt-sm">
                        <q-input
                            outlined
                            dense
                            type="number"
                            v-model.number="concurrency"
                            label="并发数"
                            placeholder="自动（默认 15）"
                            stack-label
                            min="1"
                            max="256"
                        />
                        <q-input outlined dense type="number" v-model.number="timeout" label="单域名总时限 / 秒" min="1" max="120" />
                        <q-input outlined dense type="number" v-model.number="retries" label="瞬时失败重试次数" min="0" max="5" />
                        <q-btn unelevated no-caps color="primary" type="submit" icon="play_arrow" label="创建并启动任务" :disable="!file" :loading="creating" />
                    </div>
                </q-form>
            </q-expansion-item>
        </q-card>

        <q-card flat class="workspace-card q-pa-md">
            <div class="batch-history">
                <q-select
                    outlined
                    dense
                    v-model="selectedID"
                    :disable="acting || tasks.length === 0"
                    :options="taskOptions"
                    emit-value
                    map-options
                    label="任务历史记录（含 Web / CLI / API / MCP）"
                    @update:model-value="loadSelected"
                />
                <q-btn
                    flat
                    dense
                    icon="chevron_left"
                    aria-label="上一页任务"
                    :disable="historyOffset === 0"
                    @click="changeHistoryPage(historyOffset - 100)"
                />
                <q-btn
                    flat
                    dense
                    icon="chevron_right"
                    aria-label="下一页任务"
                    :disable="tasks.length < 100"
                    @click="changeHistoryPage(historyOffset + 100)"
                />
            </div>

            <div v-if="task && task.id === selectedID" class="q-mt-md">
                <q-separator class="q-mb-md" />
                <div class="task-title">
                    <q-badge :color="stateColors[task.state] || 'primary'" class="q-px-sm q-py-xs text-subtitle2">
                        {{ stateLabels[task.state] || task.state }}
                    </q-badge>
                    <code>{{ task.id }}</code>
                    <q-btn flat dense round size="sm" icon="content_copy" aria-label="复制任务 ID" @click="copyID">
                        <q-tooltip>复制任务 ID</q-tooltip>
                    </q-btn>
                    <q-badge v-if="task.parentTaskId" outline color="blue-grey-7" class="q-px-sm">
                        重查自 {{ task.parentTaskId.slice(0, 8) }}…
                    </q-badge>
                </div>
                <p class="text-grey-8 q-mt-sm q-mb-sm">
                    模式：<strong>{{ modeLabels[task.mode] || task.mode }}</strong> · 进度：<strong>{{ task.completed }} / {{ task.total }}</strong>（{{ progressPercent }}%） · 并发 {{ task.concurrency }} · 超时 {{ task.timeout }}s · 创建于 {{ new Date(task.createdAt).toLocaleString() }}
                </p>
                <q-linear-progress :value="task.total ? task.completed / task.total : 0" size="10px" rounded color="primary" aria-label="批量查询进度" />

                <div v-if="resultOffset > 0 || nextResultOffset >= 0" class="row q-gutter-x-sm text-caption text-grey-8 q-mt-sm">
                    <strong>整个任务</strong>
                    <span v-for="(label, key) in resultLabels" :key="key">{{ label }} {{ task.counts[key] || 0 }}</span>
                </div>
                <div class="result-counts" role="group" aria-label="筛选本页结果">
                    <q-chip
                        clickable
                        dense
                        square
                        :outline="pageStatusFilter !== 'all'"
                        color="primary"
                        text-color="white"
                        @click="pageStatusFilter = 'all'"
                    >
                        本页全部 {{ results.length }}
                    </q-chip>
                    <q-chip
                        v-for="(label, key) in resultLabels"
                        :key="key"
                        clickable
                        dense
                        square
                        :outline="pageStatusFilter !== key"
                        :color="resultColors[key] || 'grey-7'"
                        text-color="white"
                        @click="pageStatusFilter = pageStatusFilter === key ? 'all' : key"
                    >
                        {{ label }} {{ pageCounts[key] || 0 }}
                    </q-chip>
                </div>

                <q-banner v-if="task.error" class="bg-red-1 text-negative q-mb-md rounded-borders">{{ task.error }}</q-banner>

                <div class="task-actions">
                    <q-btn
                        v-if="['queued', 'running'].includes(task.state)"
                        outline
                        no-caps
                        color="primary"
                        label="暂停任务"
                        icon="pause"
                        :loading="acting"
                        @click="control('pause')"
                    />
                    <q-btn
                        v-if="['paused', 'failed'].includes(task.state)"
                        unelevated
                        no-caps
                        color="primary"
                        label="继续任务"
                        icon="play_arrow"
                        :loading="acting"
                        @click="control('resume')"
                    />
                    <q-btn
                        v-if="!['completed', 'canceled'].includes(task.state)"
                        flat
                        no-caps
                        color="negative"
                        icon="cancel"
                        label="取消任务"
                        :loading="acting"
                        @click="confirmCancel"
                    />
                    <q-btn
                        v-if="!['queued', 'running'].includes(task.state) && failedCount"
                        outline
                        no-caps
                        color="primary"
                        icon="replay"
                        :label="`重查错误、超时和未知 (${failedCount})`"
                        :loading="acting"
                        @click="control('retry')"
                    />
                    <q-btn
                        outline
                        no-caps
                        color="primary"
                        icon="download"
                        label="导出已有结果 CSV"
                        :disable="!task.completed"
                        :loading="exporting"
                        @click="exportResults"
                    />
                </div>

                <p class="text-caption text-grey-7 q-mb-sm">
                    提示：未完成任务导出文件名带 <code>-partial</code>；分页按输入索引读取，点击域名可查看完整原始响应与注册期详情。
                </p>

                <q-table
                    flat
                    bordered
                    :rows="filteredPageResults"
                    :columns="columns"
                    row-key="domain"
                    :pagination="{ rowsPerPage: 0 }"
                    hide-pagination
                    :hide-bottom="filteredPageResults.length > 0"
                    :loading="loadingResults"
                    loading-label="正在读取本页结果…"
                    :no-data-label="refreshError ? '本页结果未能加载，请重试刷新。' : '当前输入区间暂无匹配结果，可切换筛选或翻页查看。'"
                >
                    <template #body-cell-domain="props">
                        <q-td :props="props">
                            <q-btn
                                flat
                                dense
                                no-caps
                                color="primary"
                                icon-right="open_in_new"
                                size="sm"
                                :label="props.row.domain"
                                @click="openDetails(props.row)"
                            />
                        </q-td>
                    </template>
                    <template #body-cell-status="props">
                        <q-td :props="props">
                            <q-badge outline :color="resultColors[props.row.status] || 'grey-8'" class="q-px-xs">
                                {{ resultLabels[props.row.status] || props.row.status }}
                            </q-badge>
                        </q-td>
                    </template>
                    <template #body-cell-expiryDate="props">
                        <q-td :props="props">{{ props.row.expiryDate ? props.row.expiryDate.slice(0, 10) : "—" }}</q-td>
                    </template>
                    <template #body-cell-domainStatus="props">
                        <q-td :props="props">{{ lifecycle(props.row) }}</q-td>
                    </template>
                </q-table>

                <div class="result-pager">
                    <q-btn
                        flat
                        no-caps
                        icon="chevron_left"
                        label="上一页结果"
                        :disable="resultOffset === 0"
                        @click="changeResultPage(false)"
                    />
                    <span class="text-grey-8">输入序号 {{ resultOffset + 1 }}–{{ nextResultOffset > resultOffset ? nextResultOffset : Math.min(resultOffset + 100, task.total) }} / 共 {{ task.total }}</span>
                    <q-btn
                        flat
                        no-caps
                        icon-right="chevron_right"
                        label="下一页结果"
                        :disable="nextResultOffset < 0"
                        @click="changeResultPage(true)"
                    />
                </div>
            </div>
            <p v-else-if="!refreshing && !refreshError" class="empty-state">
                {{ historyOffset ? "本页没有更多任务，请返回上一页。" : "还没有批量任务。请上传 TXT 文件创建任务，或通过 CLI / API / MCP 提交后在此查看。" }}
            </p>
        </q-card>

        <q-dialog :model-value="!!details" @update:model-value="details = null">
            <q-card class="result-detail">
                <q-card-section class="row items-center q-py-sm">
                    <h3>{{ details?.domain }}</h3>
                    <q-badge v-if="details?.status" :color="resultColors[details.status] || 'grey-8'" class="q-ml-sm">
                        {{ resultLabels[details.status] || details.status }}
                    </q-badge>
                    <q-space />
                    <q-btn
                        flat
                        dense
                        no-caps
                        icon="content_copy"
                        label="复制原文"
                        color="primary"
                        class="q-mr-xs"
                        :disable="!details?.rawResponse"
                        @click="copyDetailRaw"
                    />
                    <q-btn flat round dense icon="close" aria-label="关闭详情" v-close-popup />
                </q-card-section>
                <q-separator />
                <q-card-section>
                    <div class="detail-meta text-body2 q-mb-sm">
                        <div><strong>查询来源：</strong>{{ details?.source || "—" }}（{{ details?.lookupType || "—" }}）</div>
                        <div><strong>查询时间：</strong>{{ details?.queriedAt || "—" }}</div>
                        <div v-if="details?.createdDate || details?.expiryDate">
                            <strong>注册 / 到期：</strong>{{ details?.createdDate || "—" }} → {{ details?.expiryDate || "—" }}
                        </div>
                        <div v-if="details && lifecycle(details) !== '—'">
                            <strong>域名状态：</strong>{{ lifecycle(details) }}
                            <span v-if="details.rawDomainStatus?.length" class="text-grey-7">（{{ details.rawDomainStatus.join(", ") }}）</span>
                        </div>
                        <div v-if="details?.nameServer?.length"><strong>权威 DNS：</strong>{{ details.nameServer.join(", ") }}</div>
                    </div>
                    <q-banner v-if="details?.queryError" dense class="bg-orange-1 text-deep-orange-10 q-mb-sm rounded-borders">
                        {{ details.queryError }}
                    </q-banner>
                    <pre class="raw-response-box">{{ details?.rawResponse || (loadingDetail ? "正在读取原始响应…" : "无原始响应") }}</pre>
                </q-card-section>
            </q-card>
        </q-dialog>
    </section>
</template>
<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from "vue";
import { useQuasar, copyToClipboard } from "quasar";
import { api } from "boot/axios";
import WhoisSelection from "./modules/WhoisSelection.vue";
const $q = useQuasar();
const file = ref(null),
    mode = ref("whoisQuery"),
    concurrency = ref(""),
    timeout = ref(30),
    retries = ref(0);
const tasks = ref([]),
    task = ref(null),
    selectedID = ref(localStorage.getItem("powerwhois-task") || null),
    results = ref([]),
    details = ref(null),
    loadingDetail = ref(false),
    pageStatusFilter = ref("all");
const error = ref(""),
    refreshError = ref(""),
    creating = ref(false),
    refreshing = ref(false),
    acting = ref(false),
    exporting = ref(false);
const resultOffsets = ref([0]),
    nextResultOffset = ref(-1);
const loadingResults = ref(false);
let resultRequest = 0, refreshRequest = 0;
const historyOffset = ref(0),
    resultOffset = ref(0);
let timer,
    submissionKey = null,
    submissionBody = null,
    stopped = false;
const stateLabels = { queued: "等待执行", running: "查询中", paused: "已暂停", completed: "已完成", canceled: "已取消", failed: "任务失败，可继续" };
const stateColors = { queued: "blue-grey-7", running: "primary", paused: "amber-9", completed: "positive", canceled: "grey-7", failed: "negative" };
const modeLabels = { whoisQuery: "Whois (RDAP 优先)", whoisQueryWithProxy: "Whois + 代理", dnsQuery: "DNS", mixedQuery: "Mixed 混合" };
const resultLabels = { registered: "已注册", not_found: "未找到", unknown: "未知", timeout: "超时", error: "错误" };
const resultColors = { registered: "blue-8", not_found: "teal-8", unknown: "blue-grey-7", timeout: "deep-orange-8", error: "negative" };
const failedCount = computed(() => ["error", "timeout", "unknown"].reduce((n, k) => n + (task.value?.counts[k] || 0), 0));
const progressPercent = computed(() => (task.value?.total ? Math.round((task.value.completed / task.value.total) * 100) : 0));
const pageCounts = computed(() => {
    const counts = {};
    for (const row of results.value) counts[row.status] = (counts[row.status] || 0) + 1;
    return counts;
});
const filteredPageResults = computed(() =>
    pageStatusFilter.value === "all" ? results.value : results.value.filter((r) => r.status === pageStatusFilter.value)
);
const taskOptions = computed(() =>
    tasks.value.map((t) => ({
        label: `${stateLabels[t.state] || t.state} · ${modeLabels[t.mode] || t.mode} · ${t.completed}/${t.total} · ${t.id}`,
        value: t.id
    }))
);
const columns = [
    { name: "domain", label: "域名（点击查看原文）", field: "domain", align: "left" },
    { name: "status", label: "结果", field: "status", align: "left" },
    { name: "lookupType", label: "协议", field: "lookupType", align: "left" },
    { name: "expiryDate", label: "到期日", field: "expiryDate", align: "left" },
    { name: "domainStatus", label: "生命周期", field: "domainStatus", align: "left" },
    { name: "queryError", label: "说明", field: "queryError", align: "left" }
];
// Only registered results carry registry lifecycle statuses; "Unknown" on a timeout or not_found row is noise.
function lifecycle(row) {
    return row.status === "registered" && row.domainStatus ? row.domainStatus : "—";
}
function showError(e, target = error) {
    target.value = e.response?.data?.error?.message || "连接失败，请重试。已有任务和结果保留在服务器。";
}
function changeResultPage(forward) {
    if (forward) resultOffsets.value.push(nextResultOffset.value);
    else resultOffsets.value.pop();
    resultOffset.value = resultOffsets.value[resultOffsets.value.length - 1];
    results.value = [];
    nextResultOffset.value = -1;
    loadResults();
}
function changeHistoryPage(offset) {
    historyOffset.value = offset;
    tasks.value = [];
    selectedID.value = null;
    loadSelected();
}
async function loadResults() {
    if (!selectedID.value) return;
    const id = selectedID.value,
        offset = resultOffset.value,
        request = ++resultRequest;
    loadingResults.value = results.value.length === 0;
    try {
        const { data } = await api.get(`/v1/tasks/${id}/results`, { params: { offset, limit: 100, includeRaw: false } });
        if (!stopped && selectedID.value === id && resultOffset.value === offset && request === resultRequest) {
            results.value = data.items;
            nextResultOffset.value = data.nextOffset;
        }
        return true;
    } catch (e) {
        if (!stopped && selectedID.value === id && resultOffset.value === offset && request === resultRequest) showError(e, refreshError);
        return false;
    } finally {
        if (request === resultRequest) loadingResults.value = false;
    }
}
async function loadSelected() {
    resultOffset.value = 0;
    resultOffsets.value = [0];
    nextResultOffset.value = -1;
    results.value = [];
    task.value = null;
    if (selectedID.value) localStorage.setItem("powerwhois-task", selectedID.value);
    pageStatusFilter.value = "all";
    await refresh(true);
}
async function refresh(force = false) {
    if ((refreshing.value && force !== true) || stopped) return;
    const request = ++refreshRequest;
    refreshing.value = true;
    try {
        const { data } = await api.get("/v1/tasks", { params: { offset: historyOffset.value, limit: 100 } });
        if (request !== refreshRequest || stopped) return;
        tasks.value = data.items;
        if (!selectedID.value && tasks.value.length) {
            selectedID.value = tasks.value[0].id;
            localStorage.setItem("powerwhois-task", selectedID.value);
        }
        if (selectedID.value) {
            const id = selectedID.value;
            const response = await api.get(`/v1/tasks/${id}`);
            if (request !== refreshRequest || stopped || selectedID.value !== id) return;
            task.value = response.data;
            if (!(await loadResults())) return;
        }
        if (request === refreshRequest) refreshError.value = "";
    } catch (e) {
        if (request !== refreshRequest || stopped) return;
        if (e.response?.status === 404) {
            selectedID.value = null;
            task.value = null;
            results.value = [];
            localStorage.removeItem("powerwhois-task");
        }
        showError(e, refreshError);
    } finally {
        if (request === refreshRequest) refreshing.value = false;
    }
}
async function createTask() {
    if (!file.value || creating.value) return;
    if (file.value.size > 50 * 1024 * 1024) {
        error.value = "文件超过 50 MB，请拆分后上传。";
        return;
    }
    creating.value = true;
    error.value = "";
    // Blank lines are not submitted; keep each domain's file line so server errors point at the right line.
    const fileLines = [];
    try {
        const domains = [];
        (await file.value.text())
            .replace(/^\uFEFF/, "")
            .split(/\r?\n/)
            .forEach((line, index) => {
                const value = line.trim();
                if (!value) return;
                domains.push(value);
                fileLines.push(index + 1);
            });
        const body = {
            domains,
            mode: mode.value,
            concurrency: concurrency.value === "" || concurrency.value == null ? undefined : concurrency.value,
            timeout: timeout.value,
            retries: retries.value
        };
        const serialized = JSON.stringify(body);
        if (serialized !== submissionBody) {
            submissionKey = crypto.randomUUID();
            submissionBody = serialized;
        }
        const { data } = await api.post("/v1/tasks", body, { headers: { "Idempotency-Key": submissionKey }, timeout: 60000 });
        selectedID.value = data.id;
        file.value = null;
        submissionKey = null;
        submissionBody = null;
        await loadSelected();
    } catch (e) {
        showError(e);
        error.value = describeUploadError(error.value, fileLines);
    } finally {
        creating.value = false;
    }
}
function describeUploadError(message, fileLines) {
    const match = /invalid domain at line (\d+)(?::\s*(".*"))?/.exec(message || "");
    if (!match) return message;
    const item = Number(match[1]);
    let value = match[2] || "";
    try {
        value = value ? JSON.parse(value) : "";
    } catch {
        value = value.slice(1, -1);
    }
    return `文件第 ${fileLines[item - 1] || item} 行不是有效的纯域名${value ? `：“${value}”` : ""}。整批未创建，请修正或删除该行后重新上传。`;
}
async function control(action, id = selectedID.value) {
    if (acting.value || stopped || id !== selectedID.value) return;
    acting.value = true;
    try {
        const { data } = await api.post(`/v1/tasks/${id}/${action}`);
        if (selectedID.value !== id || stopped) return;
        task.value = data;
        if (action === "retry") {
            selectedID.value = data.id;
            await loadSelected();
        } else await refresh();
    } catch (e) {
        showError(e);
    } finally {
        acting.value = false;
    }
}
function confirmCancel() {
    const id = selectedID.value;
    $q.dialog({ title: "取消这个任务？", message: "已完成的结果会完整保留。取消后不可继续，但可随时导出已有结果。", cancel: "保留任务", ok: "确认取消" }).onOk(
        () => control("cancel", id)
    );
}
async function copyID() {
    try {
        await copyToClipboard(task.value.id);
        $q.notify({ message: "任务 ID 已复制", type: "positive" });
    } catch {
        error.value = "复制失败，请手动复制任务 ID。";
    }
}
async function copyDetailRaw() {
    if (!details.value?.rawResponse) return;
    try {
        await copyToClipboard(details.value.rawResponse);
        $q.notify({ message: "原始响应已复制", type: "positive" });
    } catch {
        $q.notify({ message: "复制失败，请手动选择复制", type: "warning" });
    }
}
async function openDetails(row) {
    const id = selectedID.value;
    details.value = row;
    loadingDetail.value = true;
    try {
        const { data } = await api.get(`/v1/tasks/${id}/results/${row.order}`);
        if (selectedID.value === id && details.value?.order === row.order) details.value = data;
    } catch (e) {
        showError(e);
    } finally {
        loadingDetail.value = false;
    }
}
function onVisibilityChange() {
    if (!document.hidden) refresh();
}
async function exportResults() {
    exporting.value = true;
    const { id, state } = task.value;
    const chunks = ["\ufeff"];
    let exportedCount = 0;
    try {
        for (let offset = 0; offset >= 0; ) {
            const { data, headers } = await api.get(`/v1/tasks/${id}/export`, {
                params: { format: "csv", offset, limit: 1000 },
                responseType: "text",
                timeout: 60000
            });
            chunks.push(offset === 0 ? data : data.slice(data.indexOf("\n") + 1));
            // Counts come from the server: quoted CSV fields can contain newlines.
            const count = Number(headers["x-result-count"]);
            const completed = Number(headers["x-task-completed"]);
            exportedCount += count;
            const stopped = ["completed", "canceled", "paused", "failed"].includes(headers["x-task-state"]);
            if (stopped && Number.isInteger(completed) && completed >= 0 && exportedCount >= completed) break;
            const next = Number(headers["x-next-offset"]);
            if (!Number.isInteger(next) || (next >= 0 && next <= offset)) throw new Error("invalid export cursor");
            offset = next;
        }
        const url = URL.createObjectURL(new Blob(chunks, { type: "text/csv;charset=utf-8" }));
        const link = document.createElement("a");
        link.href = url;
        link.download = `powerwhois-${id}${state === "completed" ? "" : "-partial"}.csv`;
        link.click();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
        showError(e);
    } finally {
        exporting.value = false;
    }
}
// Poll every 2 s while a listed task can still change; otherwise every 15 s to pick up CLI/API/MCP tasks.
const isActive = (t) => t?.state === "queued" || t?.state === "running";
let lastPoll = 0;
onMounted(() => {
    refresh();
    timer = setInterval(() => {
        if (document.hidden) return;
        if (!isActive(task.value) && !tasks.value.some(isActive) && Date.now() - lastPoll < 15000) return;
        lastPoll = Date.now();
        refresh();
    }, 2000);
    document.addEventListener("visibilitychange", onVisibilityChange);
});
onBeforeUnmount(() => {
    stopped = true;
    clearInterval(timer);
    document.removeEventListener("visibilitychange", onVisibilityChange);
});
</script>
<style scoped>
h2 {
    font-size: 20px;
    font-weight: 700;
    margin: 0;
    line-height: 1.35;
}
h3 {
    font-size: 17px;
    font-weight: 600;
    margin: 0;
}
.section-heading {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 16px;
    margin-bottom: 16px;
}
.section-heading p {
    color: #53677b;
    font-size: 13.5px;
    line-height: 1.6;
    margin: 4px 0 0;
}
.batch-options {
    display: grid;
    grid-template-columns: repeat(3, minmax(110px, 1fr)) auto;
    gap: 12px;
    align-items: start;
}
.batch-history {
    display: flex;
    align-items: center;
    gap: 8px;
}
.batch-history .q-select {
    flex: 1;
    min-width: 0;
}
.task-title {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
}
.task-title code {
    font-size: 12.5px;
    background: #f1f5f9;
    padding: 2px 8px;
    border-radius: 4px;
    overflow-wrap: anywhere;
}
.task-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin: 16px 0;
}
.result-counts {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin: 14px 0;
}
.result-pager {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 14px 0 2px;
}
.empty-state {
    color: #53677b;
    padding: 28px 4px;
    margin: 0;
    line-height: 1.7;
}
.result-detail {
    width: 800px;
    max-width: 94vw;
}
.detail-meta {
    display: grid;
    gap: 4px;
    color: #33475b;
}
@media (max-width: 600px) {
    .batch-options {
        grid-template-columns: 1fr 1fr;
    }
    .section-heading {
        flex-direction: column;
    }
    .result-pager {
        flex-wrap: wrap;
    }
}
</style>
