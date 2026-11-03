<template>
    <section class="batch-workspace">
        <div class="section-heading">
            <div>
                <h2>批量任务</h2>
                <p>关闭页面后仍会继续查询。Web、CLI 与 AI 助理共用这里的任务。</p>
            </div>
            <q-btn flat icon="refresh" label="刷新" :loading="refreshing" @click="refresh" />
        </div>
        <q-banner v-if="error" class="bg-red-1 text-negative q-mb-md" role="alert"
            >{{ error }}<template #action><q-btn flat label="重试" @click="refresh" /></template
        ></q-banner>
        <q-expansion-item label="新建查询任务" icon="add" default-opened header-class="text-weight-medium">
            <q-form @submit.prevent="createTask" class="q-py-md">
                <q-file
                    outlined
                    v-model="file"
                    accept=".txt,text/plain"
                    label="选择域名 TXT 文件"
                    hint="每行一个域名；自动去重。最多 100 万个域名，文件不超过 50 MB。"
                    clearable
                    :disable="creating"
                />
                <WhoisSelection v-model:queryType="mode" :disable="creating" />
                <div class="batch-options">
                    <q-input outlined dense type="number" v-model.number="concurrency" label="并发数" min="1" max="256" />
                    <q-input outlined dense type="number" v-model.number="timeout" label="每域名时限 / 秒" min="1" max="120" />
                    <q-input outlined dense type="number" v-model.number="retries" label="失败重试次数" min="0" max="5" />
                    <q-btn unelevated color="primary" type="submit" icon="play_arrow" label="开始任务" :disable="!file" :loading="creating" />
                </div>
            </q-form>
        </q-expansion-item>
        <q-separator class="q-my-lg" />
        <div class="batch-history">
            <q-select
                outlined
                v-model="selectedID"
                :disable="acting"
                :options="taskOptions"
                emit-value
                map-options
                label="任务记录（含 CLI / API / MCP）"
                @update:model-value="loadSelected"
            />
            <q-btn
                flat
                icon="chevron_left"
                aria-label="上一页任务"
                :disable="historyOffset === 0"
                @click="
                    historyOffset -= 100;
                    refresh();
                "
            />
            <q-btn
                flat
                icon="chevron_right"
                aria-label="下一页任务"
                :disable="tasks.length < 100"
                @click="
                    historyOffset += 100;
                    refresh();
                "
            />
        </div>
        <div v-if="task && task.id === selectedID" class="q-mt-lg">
            <div class="task-title">
                <h3>{{ stateLabels[task.state] || task.state }}</h3>
                <code>{{ task.id }}</code
                ><q-btn flat dense icon="content_copy" aria-label="复制任务 ID" @click="copyID" />
            </div>
            <p class="text-grey-8">
                {{ task.mode }} · {{ task.completed }} / {{ task.total }} 已完成 · 创建于 {{ new Date(task.createdAt).toLocaleString() }}
            </p>
            <q-linear-progress :value="task.total ? task.completed / task.total : 0" size="8px" color="primary" aria-label="批量查询进度" />
            <div class="result-counts" role="status">
                <span v-for="(label, key) in resultLabels" :key="key"
                    >{{ label }} <strong>{{ task.counts[key] || 0 }}</strong></span
                >
            </div>
            <q-banner v-if="task.error" class="bg-red-1 text-negative q-mb-md">{{ task.error }}</q-banner>
            <div class="task-actions">
                <q-btn
                    v-if="['queued', 'running'].includes(task.state)"
                    outline
                    color="primary"
                    label="暂停"
                    icon="pause"
                    :loading="acting"
                    @click="control('pause')"
                />
                <q-btn
                    v-if="['paused', 'failed'].includes(task.state)"
                    color="primary"
                    label="继续任务"
                    icon="play_arrow"
                    :loading="acting"
                    @click="control('resume')"
                />
                <q-btn
                    v-if="!['completed', 'canceled'].includes(task.state)"
                    flat
                    color="negative"
                    label="取消任务"
                    :loading="acting"
                    @click="confirmCancel"
                />
                <q-btn
                    v-if="!['queued', 'running'].includes(task.state) && failedCount"
                    outline
                    color="primary"
                    label="重查错误、超时和未知"
                    :loading="acting"
                    @click="control('retry')"
                />
                <q-btn outline icon="download" label="导出已有结果 CSV" :disable="!task.completed" :loading="exporting" @click="exportResults" />
            </div>
            <p class="text-caption text-grey-8">导出运行中或未完成任务时，文件名标记 partial，结果不是同一时刻的快照。结果按输入位置分页；运行中的页面可能暂时没有结果。点击域名可查看来源、时间和原始响应。</p>
            <q-table flat bordered :rows="results" :columns="columns" row-key="domain" :pagination="{ rowsPerPage: 0 }" hide-pagination hide-bottom>
                <template #body-cell-domain="props"
                    ><q-td :props="props"><q-btn flat dense no-caps color="primary" :label="props.row.domain" @click="openDetails(props.row)" /></q-td
                ></template>
                <template #body-cell-status="props"
                    ><q-td :props="props">{{ resultLabels[props.row.status] || props.row.status }}</q-td></template
                >
            </q-table>
            <div class="result-pager">
                <q-btn
                    flat
                    label="上一页结果"
                    :disable="resultOffset === 0"
                    @click="
                        resultOffsets.pop();
                        resultOffset = resultOffsets[resultOffsets.length - 1];
                        loadResults();
                    "
                /><span>{{ resultOffset + 1 }}–{{ Math.min(resultOffset + 100, task.total) }} / {{ task.total }}</span
                ><q-btn
                    flat
                    label="下一页结果"
                    :disable="nextResultOffset < 0"
                    @click="
                        resultOffsets.push(nextResultOffset);
                        resultOffset = nextResultOffset;
                        loadResults();
                    "
                />
            </div>
        </div>
        <p v-else-if="!refreshing" class="empty-state">还没有任务。上传文件创建任务，或通过 CLI / API 提交后在这里查看。</p>
        <q-dialog :model-value="!!details" @update:model-value="details = null"
            ><q-card class="result-detail"
                ><q-card-section class="row items-center"
                    ><h3>{{ details?.domain }}</h3>
                    <q-space /><q-btn flat round icon="close" aria-label="关闭详情" v-close-popup /></q-card-section
                ><q-card-section
                    ><p>来源：{{ details?.source }}<br />时间：{{ details?.queriedAt }}</p>
                    <p v-if="details?.queryError">{{ details.queryError }}</p>
                    <pre>{{ details?.rawResponse || "无原始响应" }}</pre>
                </q-card-section></q-card
            ></q-dialog
        >
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
    concurrency = ref(15),
    timeout = ref(30),
    retries = ref(0);
const tasks = ref([]),
    task = ref(null),
    selectedID = ref(localStorage.getItem("powerwhois-task") || null),
    results = ref([]),
    details = ref(null);
const error = ref(""),
    creating = ref(false),
    refreshing = ref(false),
    acting = ref(false),
    exporting = ref(false);
const resultOffsets = ref([0]), nextResultOffset = ref(-1);
let resultRequest = 0;
const historyOffset = ref(0),
    resultOffset = ref(0);
let timer,
    submissionKey = null,
    submissionBody = null,
    stopped = false;
const stateLabels = { queued: "等待执行", running: "查询中", paused: "已暂停", completed: "已完成", canceled: "已取消", failed: "任务失败，可继续" };
const resultLabels = { registered: "已注册", not_found: "未找到", unknown: "未知", timeout: "超时", error: "错误" };
const failedCount = computed(() => ["error", "timeout", "unknown"].reduce((n, k) => n + (task.value?.counts[k] || 0), 0));
const taskOptions = computed(() => tasks.value.map((t) => ({ label: `${stateLabels[t.state]} · ${t.total} 个域名 · ${t.id}`, value: t.id })));
const columns = [
    { name: "domain", label: "域名", field: "domain", align: "left" },
    { name: "status", label: "结果", field: "status", align: "left" },
    { name: "lookupType", label: "来源类型", field: "lookupType", align: "left" },
    { name: "queryError", label: "说明", field: "queryError", align: "left" }
];
function showError(e) {
    error.value = e.response?.data?.error?.message || "连接失败，请重试。已有任务和结果保留在服务器。";
}
async function loadResults() {
    if (!selectedID.value) return;
    const id = selectedID.value, offset = resultOffset.value, request = ++resultRequest;
    try {
        const { data } = await api.get(`/v1/tasks/${id}/results`, { params: { offset, limit: 100, includeRaw: false } });
        if (!stopped && selectedID.value === id && resultOffset.value === offset && request === resultRequest) {
            results.value = data.items;
            nextResultOffset.value = data.nextOffset;
        }
        return true;
    } catch (e) {
        if (!stopped && selectedID.value === id && resultOffset.value === offset && request === resultRequest) showError(e);
        return false;
    }
}
async function loadSelected() {
    resultOffset.value = 0;
    resultOffsets.value = [0];
    nextResultOffset.value = -1;
    results.value = [];
    task.value = null;
    if (selectedID.value) localStorage.setItem("powerwhois-task", selectedID.value);
    await refresh();
}
async function refresh() {
    if (refreshing.value || stopped) return;
    refreshing.value = true;
    try {
        const { data } = await api.get("/v1/tasks", { params: { offset: historyOffset.value, limit: 100 } });
        tasks.value = data.items;
        if (!selectedID.value && tasks.value.length) selectedID.value = tasks.value[0].id;
        if (selectedID.value) {
            const id = selectedID.value;
            const response = await api.get(`/v1/tasks/${id}`);
            if (selectedID.value === id) task.value = response.data;
            if (!(await loadResults())) return;
        }
        error.value = "";
    } catch (e) {
        if (e.response?.status === 404) {
            selectedID.value = null;
            task.value = null;
            results.value = [];
            localStorage.removeItem("powerwhois-task");
        }
        showError(e);
    } finally {
        refreshing.value = false;
    }
}
async function createTask() {
    if (!file.value || creating.value) return;
    if (file.value.size > 50 * 1024 * 1024) {
        error.value = "文件超过 50 MB，请拆分后上传。";
        return;
    }
    creating.value = true;
    try {
        const domains = (await file.value.text())
            .replace(/^\uFEFF/, "")
            .split(/\r?\n/)
            .map((s) => s.trim())
            .filter(Boolean);
        const body = { domains, mode: mode.value, concurrency: concurrency.value, timeout: timeout.value, retries: retries.value };
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
    } finally {
        creating.value = false;
    }
}
async function control(action) {
    acting.value = true;
    const id = selectedID.value;
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
    $q.dialog({ title: "取消这个任务？", message: "已有结果会保留。取消后不能继续，可创建新任务。", cancel: "保留任务", ok: "取消任务" }).onOk(() =>
        control("cancel")
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
async function openDetails(row) {
    const id = selectedID.value;
    details.value = row;
    try {
        const { data } = await api.get(`/v1/tasks/${id}/results/${row.order}`);
        if (selectedID.value === id && details.value?.order === row.order) details.value = data;
    } catch (e) { showError(e); }
}
async function exportResults() {
    exporting.value = true;
    const { id, state } = task.value;
    const chunks = ["\ufeff"];
    try {
        for (let offset = 0; offset >= 0;) {
            const { data, headers } = await api.get(`/v1/tasks/${id}/export`, {
                params: { format: "csv", offset, limit: 1000 },
                responseType: "text",
                timeout: 60000
            });
            chunks.push(offset === 0 ? data : data.slice(data.indexOf("\n") + 1));
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
onMounted(() => {
    refresh();
    timer = setInterval(refresh, 2000);
});
onBeforeUnmount(() => {
    stopped = true;
    clearInterval(timer);
});
</script>
<style scoped>
h2 {
    font-size: 22px;
    font-weight: 650;
    margin: 0;
    line-height: 1.4;
}
h3 {
    font-size: 18px;
    font-weight: 600;
    margin: 0;
}
.section-heading {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 16px;
    margin-bottom: 20px;
}
.section-heading p {
    color: #53677b;
    line-height: 1.7;
    margin: 8px 0 0;
}
.batch-options {
    display: grid;
    grid-template-columns: repeat(3, minmax(100px, 1fr)) auto;
    gap: 16px;
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
    gap: 12px;
}
.task-title code {
    font-size: 12px;
    overflow-wrap: anywhere;
}
.task-actions,
.result-counts {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    margin: 20px 0;
}
.result-counts strong {
    font-variant-numeric: tabular-nums;
    margin-left: 6px;
}
.result-pager {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 16px 0;
}
.empty-state {
    color: #53677b;
    padding: 40px 0;
    line-height: 1.8;
}
.result-detail {
    width: 800px;
    max-width: 94vw;
}
.result-detail pre {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-height: 55vh;
    overflow: auto;
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
