<template>
    <q-layout class="workspace-layout"
        ><q-page-container
            ><q-page>
                <a class="skip-link" href="#main">跳到工作区</a>
                <header class="app-header">
                    <div class="brand">
                        <q-icon name="travel_explore" size="28px" color="primary" /><strong>PowerWhois</strong><span>域名查询工作台</span>
                    </div>
                    <span class="connection">通过本地安全连接访问</span>
                </header>
                <div class="workspace">
                    <aside class="sidebar" aria-label="作业导航">
                        <div class="section-title">
                            <h2>我的作业</h2>
                            <q-btn flat dense icon="refresh" aria-label="刷新作业列表" :disable="busy" @click="list()" />
                        </div>
                        <q-btn outline color="primary" icon="add" label="导入新作业" class="full-width q-my-md" :disable="busy" @click="newJob" />
                        <q-form @submit="lookup"
                            ><q-input v-model.trim="lookupID" outlined dense label="按作业 ID 查找" :disable="busy"
                                ><template #append
                                    ><q-btn
                                        flat
                                        round
                                        dense
                                        icon="search"
                                        type="submit"
                                        aria-label="查找作业"
                                        :disable="busy || !lookupID" /></template></q-input
                        ></q-form>
                        <p v-if="!jobs.length && !busy" class="muted q-mt-lg">还没有作业。导入一份 XLSX 开始查询。</p>
                        <q-list class="job-list"
                            ><q-item
                                v-for="entry in jobs"
                                :key="entry.id"
                                clickable
                                :active="job?.id === entry.id"
                                active-class="selected-job"
                                :disable="busy"
                                @click="selectJob(entry.id)"
                                ><q-item-section
                                    ><q-item-label class="job-source">{{ entry.source || "未命名工作簿" }}</q-item-label
                                    ><q-item-label caption
                                        >{{ stateLabel(entry.state) }} · {{ number(entry.completed) }} / {{ number(entry.candidates) }}</q-item-label
                                    ><q-item-label caption class="job-id">{{ entry.id }}</q-item-label></q-item-section
                                ></q-item
                            ></q-list
                        >
                        <q-btn v-if="jobsNext" flat label="更多作业" class="full-width" :disable="busy" @click="list(jobsNext)" />
                        <details class="automation-help">
                            <summary>使用 CLI / API 自动化</summary>
                            <p>扫描在服务端持续执行，可以关闭页面。CLI 输出 JSON，适合代理提交和跟踪任务。</p>
                            <code>powerwhois jobs</code><code>powerwhois status --job &lt;ID&gt;</code
                            ><a href="/api/v1/health" target="_blank" rel="noopener">查看服务健康状态</a>
                        </details>
                    </aside>
                    <main id="main" tabindex="-1" class="main-content" :aria-busy="busy">
                        <div class="page-heading">
                            <div>
                                <h1>{{ job ? "作业详情" : "新建域名查询作业" }}</h1>
                                <p>{{ job ? job.source : "导入 Excel，生成拼写变体，预览后再开始查询。" }}</p>
                            </div>
                            <q-btn v-if="job" flat icon="refresh" label="刷新" :disable="busy" @click="refresh" />
                        </div>
                        <q-banner v-if="error" class="error-banner q-mb-md" role="alert"
                            ><template #avatar><q-icon name="error_outline" /></template>{{ error
                            }}<template #action><q-btn flat label="重试刷新" :disable="busy" @click="refresh" /></template
                        ></q-banner>
                        <q-linear-progress v-if="busy" indeterminate color="primary" class="q-mb-md" aria-label="正在处理请求" />
                        <section v-if="!job" class="panel import-panel" aria-labelledby="import-title">
                            <h2 id="import-title">导入原始域名</h2>
                            <p class="muted">支持 .xlsx，最大 32 MiB。每行一个完整域名，例如 example.com。</p>
                            <q-form @submit="importFile"
                                ><q-file v-model="file" outlined label="选择 XLSX 文件" accept=".xlsx" :disable="busy" clearable
                                    ><template #prepend><q-icon name="upload_file" /></template
                                ></q-file>
                                <div class="import-options">
                                    <q-input v-model="sheet" outlined dense label="工作表名称" hint="留空使用第一张工作表" :disable="busy" /><q-input
                                        v-model.trim="column"
                                        outlined
                                        dense
                                        label="域名所在列"
                                        placeholder="B"
                                        :disable="busy"
                                    /><q-checkbox v-model="header" label="首行是标题" :disable="busy" />
                                </div>
                                <q-btn
                                    unelevated
                                    color="primary"
                                    icon="file_upload"
                                    label="导入 XLSX"
                                    type="submit"
                                    :loading="busy"
                                    :disable="!file || !column"
                                />
                            </q-form>
                            <div class="import-note">
                                <q-icon name="info_outline" size="20px" />
                                <p>导入和生成候选不会发起域名查询。你可以先检查无效输入和候选数量，再决定是否扫描。</p>
                            </div>
                        </section>
                        <template v-else>
                            <section class="panel job-summary" aria-labelledby="summary-title">
                                <div class="section-title">
                                    <h2 id="summary-title">{{ stateLabel(job.state) }}</h2>
                                    <span class="state-badge" :class="job.state">{{ job.state }}</span>
                                </div>
                                <p class="job-id selectable">{{ job.id }}</p>
                                <dl class="job-facts">
                                    <div>
                                        <dt>输入行数</dt>
                                        <dd>{{ number(job.inputRows) }}</dd>
                                    </div>
                                    <div>
                                        <dt>有效唯一域名</dt>
                                        <dd>{{ number(job.uniqueSeeds) }}</dd>
                                    </div>
                                    <div>
                                        <dt>无效输入</dt>
                                        <dd :class="{ 'text-negative': job.invalidRows }">{{ number(job.invalidRows) }}</dd>
                                    </div>
                                    <div>
                                        <dt>唯一候选</dt>
                                        <dd>{{ number(job.candidates) }}</dd>
                                    </div>
                                </dl>
                                <q-banner v-if="job.error" class="error-banner q-my-md" role="alert">{{ job.error }}</q-banner>
                                <q-banner v-if="job.truncated" class="warning-banner q-my-md"
                                    >已达到候选上限，当前候选只覆盖部分生成结果。需要更多候选时，请重新导入并提高上限。</q-banner
                                >
                                <q-banner v-if="job.invalidRows" class="warning-banner q-my-md"
                                    >{{ number(job.invalidRows) }} 行输入无效。请在“原始输入”中查看具体原因。</q-banner
                                >
                                <q-form v-if="['draft', 'generating'].includes(job.state)" class="generation-form" @submit="act('generate')">
                                    <q-select
                                        v-model="rules"
                                        :options="ruleOptions"
                                        emit-value
                                        map-options
                                        multiple
                                        use-chips
                                        outlined
                                        label="拼写变体规则"
                                        :disable="busy || job.state === 'generating'"
                                    />
                                    <div class="generation-options">
                                        <q-input
                                            v-model.number="cap"
                                            type="number"
                                            min="1"
                                            max="2000000"
                                            outlined
                                            dense
                                            label="唯一候选上限"
                                            :disable="busy || job.state === 'generating'"
                                        /><q-input
                                            v-if="rules.includes('tld')"
                                            v-model="tlds"
                                            outlined
                                            dense
                                            label="替换后缀（逗号分隔）"
                                            :disable="busy || job.state === 'generating'"
                                        />
                                    </div>
                                    <q-btn
                                        unelevated
                                        color="primary"
                                        :label="job.state === 'generating' ? '继续生成候选' : '生成候选并预览'"
                                        type="submit"
                                        :disable="busy || !rules.length || !job.uniqueSeeds || !Number.isInteger(cap) || cap < 1 || cap > 2000000"
                                    /><q-btn
                                        v-if="job.state === 'draft'"
                                        flat
                                        color="negative"
                                        label="取消作业"
                                        :disable="busy"
                                        @click="act('cancel')"
                                    />
                                    <p class="muted q-mt-sm">生成参数保存后不可修改；更换规则需要新建作业。</p>
                                </q-form>
                                <div v-else class="scan-controls">
                                    <div class="progress-label" aria-live="polite">
                                        <span>已检查 {{ number(job.completed) }} / {{ number(job.candidates) }}</span
                                        ><span>{{ percentage }}%</span>
                                    </div>
                                    <q-linear-progress
                                        :value="job.candidates ? job.completed / job.candidates : 0"
                                        size="7px"
                                        color="primary"
                                        rounded
                                        aria-label="域名查询进度"
                                    />
                                    <div class="result-counts">
                                        <span v-for="status in statuses" :key="status" :class="'result-' + status"
                                            >{{ stateLabel(status) }} <strong>{{ number(job.counts?.[status] || 0) }}</strong></span
                                        ><span>{{ job.partial ? "部分结果" : "已完成全部查询" }}</span>
                                    </div>
                                    <div class="action-bar">
                                        <q-btn
                                            v-for="action in allowedActions"
                                            :key="action"
                                            :label="actionLabels[action]"
                                            :color="action === 'cancel' ? 'negative' : 'primary'"
                                            :outline="action !== 'start' && action !== 'resume'"
                                            :unelevated="action === 'start' || action === 'resume'"
                                            :disable="busy"
                                            @click="act(action)"
                                        /><span v-if="job.state === 'running'" class="muted">进度每 3 秒自动更新，可关闭页面。</span>
                                    </div>
                                </div>
                            </section>
                            <section class="panel results-panel" aria-label="作业数据">
                                <div class="data-toolbar">
                                    <q-tabs
                                        :model-value="pageKind"
                                        dense
                                        no-caps
                                        align="left"
                                        active-color="primary"
                                        indicator-color="primary"
                                        @update:model-value="changePage"
                                        ><q-tab name="items" label="候选与结果" :disable="busy" /><q-tab
                                            name="seeds"
                                            label="原始输入"
                                            :disable="busy" /><q-tab name="origins" label="生成来源" :disable="busy" /></q-tabs
                                    ><q-btn
                                        flat
                                        color="primary"
                                        icon="download"
                                        label="导出 XLSX"
                                        :disable="busy"
                                        :href="`/api/v1/jobs/${job.id}/export?denominator=${denominator}`"
                                        :download="`powerwhois-${job.id}.xlsx`"
                                    />
                                </div>
                                <div class="export-options">
                                    <q-select
                                        v-model="denominator"
                                        :options="denominatorOptions"
                                        emit-value
                                        map-options
                                        dense
                                        borderless
                                        label="导出比例分母"
                                    /><span class="muted">每页最多 100 条；导出包含完整作业。</span>
                                </div>
                                <q-markup-table flat wrap-cells class="data-table"
                                    ><caption class="sr-only">
                                        {{
                                            pageTitles[pageKind]
                                        }}，第
                                        {{
                                            cursors.length
                                        }}
                                        页
                                    </caption>
                                    <thead>
                                        <tr>
                                            <th v-for="key in columns" :key="key" scope="col">{{ labels[key] }}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        <tr v-for="(item, index) in items" :key="index">
                                            <td v-for="key in columns" :key="key">
                                                <span v-if="key === 'status'" class="status-dot" :class="'result-' + item.status">{{
                                                    stateLabel(item.status)
                                                }}</span
                                                ><q-btn
                                                    v-else-if="key === 'evidence'"
                                                    flat
                                                    dense
                                                    color="primary"
                                                    label="查看证据"
                                                    :disable="!item.result"
                                                    @click="detail = item.result"
                                                /><template v-else>{{ display(item[key]) }}</template>
                                            </td>
                                        </tr>
                                        <tr v-if="!items.length">
                                            <td :colspan="columns.length" class="table-empty">
                                                {{
                                                    pageKind === "items" && job.state === "draft"
                                                        ? "尚未生成候选。选择规则后，点击“生成候选并预览”。"
                                                        : "当前没有记录。"
                                                }}
                                            </td>
                                        </tr>
                                    </tbody>
                                </q-markup-table>
                                <div class="pagination">
                                    <span class="muted">第 {{ cursors.length }} 页 · 本页 {{ items.length }} 条</span>
                                    <div>
                                        <q-btn flat label="上一页" :disable="busy || cursors.length === 1" @click="previousPage" /><q-btn
                                            flat
                                            label="下一页"
                                            :disable="busy || !next"
                                            @click="nextPage"
                                        />
                                    </div>
                                </div>
                            </section>
                        </template>
                        <p class="registry-note">
                            <q-icon name="verified_user" size="18px" />查询通过注册局 RDAP
                            服务。未找到（not_found）表示没有查到记录，不保证域名可购买。
                        </p>
                    </main>
                </div>
                <q-dialog :model-value="!!detail" @update:model-value="detail = null"
                    ><q-card class="evidence-dialog"
                        ><q-card-section class="section-title"
                            ><h2>查询证据</h2>
                            <q-btn v-close-popup flat round dense icon="close" aria-label="关闭查询证据" /></q-card-section
                        ><q-card-section>
                            <pre>{{ JSON.stringify(detail, null, 2) }}</pre>
                        </q-card-section></q-card
                    ></q-dialog
                >
            </q-page></q-page-container
        ></q-layout
    >
</template>
<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useQuasar } from "quasar";
import { actionsFor, columnsFor, stateLabel, ruleOptions, savedOptions } from "src/utils/workflowView";
const $q = useQuasar();
const file = ref(null),
    sheet = ref(""),
    column = ref("B"),
    header = ref(true),
    lookupID = ref("");
const job = ref(null),
    jobs = ref([]),
    jobsNext = ref(""),
    busy = ref(false),
    error = ref(""),
    items = ref([]),
    next = ref(""),
    pageKind = ref("items"),
    cursors = ref([""]),
    detail = ref(null);
const rules = ref(["omission", "repetition", "transposition"]),
    cap = ref(500000),
    tlds = ref("com,net,org,co"),
    denominator = ref("all");
const statuses = ["registered", "not_found", "unknown", "error"];
const actionLabels = { start: "开始查询", pause: "暂停查询", resume: "继续查询", cancel: "取消作业" };
const pageTitles = { items: "候选与结果", seeds: "原始输入", origins: "生成来源" };
const denominatorOptions = [
    { label: "全部候选", value: "all" },
    { label: "仅已知结果（已注册 + 未找到）", value: "known" }
];
const labels = {
    domain: "域名",
    status: "查询状态",
    checkedAt: "检查时间（本地）",
    httpStatus: "HTTP",
    error: "错误 / 无效原因",
    evidence: "原始证据",
    sheet: "工作表",
    row: "行号",
    raw: "原始值",
    duplicate: "重复输入",
    candidate: "候选域名",
    rule: "生成规则"
};
const columns = computed(() => columnsFor(pageKind.value)),
    allowedActions = computed(() => actionsFor(job.value));
const percentage = computed(() => (job.value?.candidates ? ((job.value.completed / job.value.candidates) * 100).toFixed(1) : "0"));
const number = (value) => new Intl.NumberFormat("zh-CN").format(value);
const display = (value) => (typeof value === "boolean" ? (value ? "是" : "否") : String(value ?? "—"));
let timer;
async function request(path, options) {
    const r = await fetch("/api/v1/" + path, options);
    if (!r.headers.get("Content-Type")?.includes("application/json")) throw new Error("服务未返回有效数据，请检查服务或连接后重试。");
    const d = await r.json();
    if (!r.ok) throw new Error(d.error || `请求失败（HTTP ${r.status}）`);
    return d;
}
function scheduleRefresh() {
    clearTimeout(timer);
    if (["running", "generating", "importing"].includes(job.value?.state)) timer = setTimeout(refresh, 3000);
}
async function run(fn) {
    if (busy.value) return;
    clearTimeout(timer);
    busy.value = true;
    error.value = "";
    try {
        await fn();
    } catch (e) {
        error.value = e.message;
    } finally {
        busy.value = false;
        scheduleRefresh();
    }
}
async function fetchJobs(cursor = "") {
    const d = await request("jobs?limit=100&after=" + encodeURIComponent(cursor));
    jobs.value = cursor ? [...jobs.value, ...d.items] : d.items;
    jobsNext.value = d.next;
}
async function fetchPage(kind = pageKind.value, cursor = cursors.value[cursors.value.length - 1]) {
    const d = await request(`jobs/${job.value.id}/${kind}?limit=100&after=${encodeURIComponent(cursor)}`);
    items.value =
        kind === "items"
            ? d.items.map((item) => ({
                  ...item,
                  status: item.result?.status || "pending",
                  checkedAt: item.result?.checkedAt ? new Date(item.result.checkedAt).toLocaleString() : "—",
                  httpStatus: item.result?.httpStatus || "—",
                  error: item.result?.error
              }))
            : d.items;
    next.value = d.next;
}
async function list(cursor = "") {
    await run(() => fetchJobs(cursor));
}
async function selectJob(id) {
    await run(async () => {
        const selected = await request("jobs/" + encodeURIComponent(id));
        job.value = selected;
        lookupID.value = id;
        pageKind.value = "items";
        cursors.value = [""];
        items.value = [];
        next.value = "";
        const o = savedOptions(selected);
        rules.value = o.rules;
        cap.value = o.maxCandidates;
        tlds.value = o.tlds.join(",");
        await fetchPage();
    });
}
async function lookup() {
    if (lookupID.value) await selectJob(lookupID.value);
}
function newJob() {
    clearTimeout(timer);
    job.value = null;
    error.value = "";
    items.value = [];
    next.value = "";
}
async function refresh() {
    await run(async () => {
        if (job.value) {
            job.value = await request("jobs/" + job.value.id);
            await fetchPage();
        }
        await fetchJobs();
    });
}
async function importFile() {
    await run(async () => {
        if (!file.value || !/\.xlsx$/i.test(file.value.name) || file.value.size > 32 * 1024 * 1024)
            throw new Error("请选择不超过 32 MiB 的 .xlsx 文件。");
        const form = new FormData();
        form.append("file", file.value);
        form.append("sheet", sheet.value);
        form.append("column", column.value);
        form.append("header", String(header.value));
        job.value = await request("jobs/import", { method: "POST", body: form });
        rules.value = ["omission", "repetition", "transposition"];
        cap.value = 500000;
        tlds.value = "com,net,org,co";
        pageKind.value = "seeds";
        cursors.value = [""];
        file.value = null;
        await fetchPage();
        await fetchJobs();
    });
}
async function act(action) {
    if (action === "cancel") {
        $q.dialog({
            title: "取消这个作业？",
            message: "已查结果会保留，取消后无法继续扫描。若只是暂时停止，请使用暂停。",
            cancel: { label: "返回" },
            ok: { label: "确认取消", color: "negative" }
        }).onOk(() => performAction(action));
    } else await performAction(action);
}
async function performAction(action) {
    await run(async () => {
        const options =
            action === "generate"
                ? {
                      rules: rules.value,
                      maxCandidates: cap.value,
                      tlds: tlds.value
                          .split(",")
                          .map((s) => s.trim())
                          .filter(Boolean),
                      includeSeeds: job.value.options.includeSeeds
                  }
                : {};
        job.value = await request(`jobs/${job.value.id}/${action}`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(options)
        });
        if (action === "generate") {
            pageKind.value = "items";
            cursors.value = [""];
        }
        await fetchPage();
        await fetchJobs();
    });
}
async function changePage(kind) {
    await run(async () => {
        await fetchPage(kind, "");
        pageKind.value = kind;
        cursors.value = [""];
    });
}
async function nextPage() {
    await run(async () => {
        const cursor = next.value;
        await fetchPage(pageKind.value, cursor);
        cursors.value.push(cursor);
    });
}
async function previousPage() {
    await run(async () => {
        await fetchPage(pageKind.value, cursors.value[cursors.value.length - 2]);
        cursors.value.pop();
    });
}
onMounted(() => list());
onUnmounted(() => clearTimeout(timer));
</script>
<style scoped>
.workspace-layout {
    background: #f5f7fa;
    color: #243247;
    font-family: system-ui, -apple-system, "Segoe UI", "Microsoft YaHei", sans-serif;
    font-size: 14px;
}
.app-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 72px;
    padding: 16px 32px;
    background: #fff;
    border-bottom: 1px solid #dce3ec;
    gap: 16px;
}
.brand {
    display: flex;
    align-items: center;
    gap: 12px;
}
.brand strong {
    font-size: 21px;
    letter-spacing: -0.025em;
}
.brand span,
.connection,
.muted {
    color: #526279;
}
.connection {
    font-size: 12px;
}
.workspace {
    display: grid;
    grid-template-columns: 280px minmax(0, 1fr);
    max-width: 1520px;
    margin: auto;
    min-height: calc(100vh - 73px);
}
.sidebar {
    padding: 24px 20px;
    border-right: 1px solid #dce3ec;
    background: #fbfcfe;
}
.section-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
}
.section-title h2 {
    margin: 0;
}
.job-list {
    margin-top: 16px;
}
.job-list .q-item {
    padding: 12px 10px;
    border-radius: 8px;
    margin: 4px 0;
}
.job-source {
    font-weight: 600;
    overflow-wrap: anywhere;
}
.selected-job {
    color: #174c91;
    background: #e7effa;
}
.job-id {
    font: 12px/1.6 ui-monospace, Consolas, monospace;
    overflow-wrap: anywhere;
    color: #526279;
}
.job-list .job-id {
    font-size: 11px;
}
.selectable {
    user-select: all;
}
.main-content {
    padding: 32px;
    min-width: 0;
}
.page-heading {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 16px;
    margin-bottom: 24px;
}
.page-heading h1 {
    font-size: 25px;
    line-height: 1.4;
    font-weight: 650;
    letter-spacing: -0.02em;
    margin: 0 0 4px;
}
.page-heading p {
    margin: 0;
    color: #526279;
    overflow-wrap: anywhere;
}
h2 {
    font-size: 17px;
    line-height: 1.5;
    font-weight: 600;
    margin: 0 0 12px;
}
p {
    line-height: 1.7;
}
.panel {
    background: #fff;
    border: 1px solid #dce3ec;
    border-radius: 12px;
    margin-bottom: 20px;
}
.import-panel,
.job-summary {
    padding: 24px;
}
.import-panel {
    max-width: 900px;
}
.import-options {
    display: grid;
    grid-template-columns: minmax(180px, 1fr) 140px 160px;
    align-items: start;
    gap: 16px;
    margin: 24px 0;
}
.import-note {
    display: flex;
    gap: 10px;
    margin-top: 28px;
    padding-top: 20px;
    border-top: 1px solid #e5eaf1;
    color: #526279;
}
.import-note p {
    margin: 0;
    max-width: 70ch;
}
.job-facts {
    display: flex;
    flex-wrap: wrap;
    gap: 20px 40px;
    margin: 22px 0;
}
.job-facts dt {
    color: #526279;
    font-size: 12px;
}
.job-facts dd {
    margin: 4px 0 0;
    font-size: 20px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
}
.state-badge {
    font-size: 12px;
    padding: 4px 9px;
    border-radius: 5px;
    background: #edf1f6;
    color: #42536a;
}
.state-badge.running {
    background: #e7effa;
    color: #174c91;
}
.state-badge.completed {
    background: #e6f3ed;
    color: #1b6448;
}
.state-badge.error {
    background: #fcebec;
    color: #ad2537;
}
.generation-form {
    border-top: 1px solid #e5eaf1;
    padding-top: 20px;
}
.generation-options {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    margin: 16px 0;
}
.generation-options .q-input {
    min-width: 200px;
    flex: 1;
}
.progress-label {
    display: flex;
    justify-content: space-between;
    margin-bottom: 10px;
    font-variant-numeric: tabular-nums;
}
.result-counts {
    display: flex;
    flex-wrap: wrap;
    gap: 12px 24px;
    margin: 16px 0;
    font-size: 13px;
}
.result-counts strong {
    margin-left: 4px;
    font-variant-numeric: tabular-nums;
}
.action-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    margin-top: 20px;
}
.result-registered {
    color: #1b6448;
}
.result-not_found {
    color: #174c91;
}
.result-unknown,
.result-pending {
    color: #526279;
}
.result-error {
    color: #ad2537;
}
.status-dot {
    font-weight: 600;
    white-space: nowrap;
}
.data-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    border-bottom: 1px solid #e5eaf1;
    padding: 4px 12px;
}
.export-options {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 8px 20px;
}
.export-options .q-select {
    min-width: 180px;
}
.data-table th {
    background: #f6f8fb;
    text-align: left;
    color: #42536a;
    font-weight: 600;
}
.data-table td {
    max-width: 350px;
    overflow-wrap: anywhere;
    white-space: normal;
}
.table-empty {
    padding: 40px 20px !important;
    text-align: center;
    color: #526279;
}
.pagination {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 16px;
    border-top: 1px solid #e5eaf1;
}
.error-banner {
    background: #fcebec;
    color: #912439;
    border-radius: 8px;
}
.warning-banner {
    background: #fff5df;
    color: #77501b;
    border-radius: 8px;
}
.registry-note {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    color: #526279;
    font-size: 12px;
    margin: 24px 0;
}
.registry-note .q-icon {
    flex-shrink: 0;
    margin-top: 2px;
}
.automation-help {
    margin-top: 32px;
    font-size: 12px;
    color: #526279;
}
.automation-help summary {
    cursor: pointer;
    font-weight: 600;
    padding: 8px 0;
}
.automation-help code {
    display: block;
    overflow-wrap: anywhere;
    margin: 8px 0;
}
.automation-help a {
    color: #174c91;
}
.evidence-dialog {
    width: 850px;
    max-width: 95vw;
}
.evidence-dialog pre {
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font: 13px/1.7 ui-monospace, Consolas, monospace;
    max-height: 65vh;
    overflow: auto;
}
.skip-link {
    position: absolute;
    left: 12px;
    top: -80px;
    z-index: 100;
    background: #fff;
    padding: 12px;
    color: #174c91;
}
.skip-link:focus {
    top: 8px;
}
.sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
}
:deep(:focus-visible) {
    outline: 2px solid #174c91;
    outline-offset: 3px;
}
:deep(.q-btn) {
    text-transform: none;
}
:deep(.q-field__label),
:deep(.q-item__label--caption) {
    color: #526279;
}
::selection {
    background: #cbdcf6;
    color: #153f76;
}
@media (max-width: 1000px) {
    .workspace {
        grid-template-columns: 240px minmax(0, 1fr);
    }
    .main-content {
        padding: 24px;
    }
    .app-header {
        padding: 16px 24px;
    }
    .import-options {
        grid-template-columns: minmax(120px, 1fr) 110px;
    }
    .import-options .q-checkbox {
        grid-column: 1/-1;
    }
    .data-toolbar,
    .export-options {
        flex-wrap: wrap;
    }
    .job-facts {
        gap: 16px 28px;
    }
}
@media (max-width: 700px) {
    .app-header {
        padding: 16px;
    }
    .brand {
        gap: 8px;
        flex-wrap: wrap;
    }
    .brand span {
        display: none;
    }
    .connection {
        max-width: 110px;
        text-align: right;
    }
    .workspace {
        display: flex;
        flex-direction: column;
    }
    .sidebar {
        border-right: 0;
        border-bottom: 1px solid #dce3ec;
        padding: 16px;
    }
    .job-list {
        max-height: 220px;
        overflow: auto;
    }
    .automation-help {
        margin-top: 12px;
    }
    .main-content {
        padding: 20px 16px;
    }
    .page-heading {
        align-items: flex-start;
        margin-bottom: 20px;
    }
    .page-heading h1 {
        font-size: 22px;
    }
    .import-panel,
    .job-summary {
        padding: 20px 16px;
    }
    .import-options {
        grid-template-columns: minmax(0, 1fr) 100px;
        gap: 12px;
    }
    .data-toolbar {
        padding: 8px;
    }
    .data-toolbar .q-tabs {
        width: 100%;
    }
    .export-options {
        padding: 8px 16px;
    }
    .data-table td {
        min-width: 90px;
    }
    .pagination {
        padding: 8px;
        gap: 8px;
        font-size: 12px;
    }
    .job-facts {
        gap: 16px 24px;
    }
}
</style>
