<template>
    <header class="page-heading">
        <h1>即时域名查询</h1>
        <p>查询域名注册状态、到期日期与权威 DNS，逐条查看注册局响应。</p>
    </header>
    <q-card flat class="workspace-card lookup-card">
        <div class="lookup-grid">
            <section class="domain-editor">
                <div class="input-heading">
                    <label for="web-domains" class="section-label">域名列表</label>
                    <div class="input-tools">
                        <q-btn flat dense no-caps icon="auto_fix_high" label="整理去重" :disable="webStore.busy || !domainInput" @click="cleanDomainInput" />
                        <q-btn flat dense no-caps icon="clear_all" label="清空" :disable="webStore.busy || !domainInput" @click="domainInput = ''" />
                    </div>
                </div>
                <q-input id="web-domains" outlined v-model="domainInput" type="textarea" rows="4" class="domain-textarea"
                    aria-label="查询域名列表" aria-describedby="domain-format" autocomplete="off" autocapitalize="none" spellcheck="false"
                    placeholder="example.com&#10;example.org&#10;例子.中国" :disable="webStore.busy"
                    @keydown.ctrl.enter.prevent="queryDomains()" @keydown.meta.enter.prevent="queryDomains()" />
                <div class="editor-footnote">
                    <span id="domain-format">每行一个域名，支持中文域名。</span>
                    <span :class="{ 'text-negative': uniqueInputCount > settingStore.webCheckDomainLimit }">
                        已输入 {{ uniqueInputCount }} / {{ settingStore.webCheckDomainLimit }}
                    </span>
                </div>
            </section>
            <aside class="query-sidebar">
                <WhoisSelection v-model:queryType="queryType" :disable="webStore.busy" />
                <RegisterSelection v-if="tokenStore.token && settingStore.registerApis.length" v-model:registerType="registerType" :disable="webStore.busy" />
            </aside>
        </div>
        <div class="form-action-bar">
            <span class="keyboard-hint"><kbd>Ctrl</kbd> / <kbd>⌘</kbd> + <kbd>Enter</kbd> 开始查询</span>
            <div class="form-buttons">
                <q-btn v-if="pendingCount > 0" outline no-caps color="negative" icon="stop_circle" label="停止查询" @click="stopQuery"
                    :loading="webStore.pendingQuery === 'stop'" />
                <q-btn unelevated no-caps color="primary" icon="search" label="开始查询" class="primary-action"
                    :loading="webStore.pendingQuery === 'webCheck' || pendingCount > 0" :percentage="handleProgress" :disable="webStore.busy" @click="queryDomains()">
                    <template #loading>
                        <q-spinner class="on-left" />
                        <span>{{ webStore.pendingQuery === "webCheck" ? "正在提交…" : `查询中 ${handleProgress}%` }}</span>
                    </template>
                </q-btn>
            </div>
        </div>
        <p v-if="otherTabBusy" class="busy-message" role="status">Typo 查询正在进行。完成后即可开始新的网页查询。</p>
    </q-card>

    <CheckedResult v-if="webCheckDomains.length" :domainResults="webCheckDomains" :resultType="resultType" :queryType="queryType" :registerType="registerType" />
    <section v-else class="results-placeholder" aria-label="查询结果">
        <q-icon name="manage_search" size="32px" aria-hidden="true" /><div><h2>查询结果</h2><p>提交后，域名状态与查询详情将在这里显示。</p></div>
    </section>
</template>

<script setup>
defineOptions({
    name: "WebCheck"
});

import { ref, computed, watchEffect } from "vue";
import { useQuasar } from "quasar";
import { submitQuery, stopQuery } from "src/utils/websocketHandler";
import { isPlainDomainInput, normalizeDomainInput } from "src/utils/domainInput";
import { useTokenStore } from "src/stores/tokenStore";
import { useWebStore } from "src/stores/webStore";
import { useSettingStore } from "src/stores/settingStore";

import WhoisSelection from "src/components/modules/WhoisSelection.vue";
import RegisterSelection from "src/components/modules/RegisterSelection.vue";
import CheckedResult from "src/components/modules/CheckedResult.vue";

const $q = useQuasar();

const webCheckDomains = ref([]);

const tokenStore = useTokenStore();
const webStore = useWebStore();

const settingStore = useSettingStore();

const domainInput = computed({
    get: () => webStore.webDomainInput,
    set: (val) => {
        webStore.webDomainInput = val;
    }
});
const queryType = computed({
    get: () => webStore.webQueryType,
    set: (val) => {
        webStore.webQueryType = val;
    }
});
const registerType = ref(null);

const resultType = ref("webCheck");

watchEffect(() => {
    webCheckDomains.value = webStore.domains.filter((domain) => domain.checkType == "webCheck");
});

function parseRawEntries(text) {
    if (!text) return [];
    const entries = [];
    text.replace(/^\uFEFF/, "")
        .split(/\r?\n/)
        .forEach((raw, index) => {
            const value = normalizeDomainInput(raw);
            if (value) entries.push({ value, line: index + 1 });
        });
    return entries;
}

const uniqueInputCount = computed(() => {
    const entries = parseRawEntries(domainInput.value);
    return new Set(entries.map((d) => d.value)).size;
});

function cleanDomainInput() {
    const entries = parseRawEntries(domainInput.value);
    const deduped = [...new Set(entries.map((d) => d.value))];
    const removed = entries.length - deduped.length;
    domainInput.value = deduped.join("\n");
    $q.notify({
        position: "top",
        type: "positive",
        message: removed > 0 ? `已整理完成，移除 ${removed} 个重复或空白项` : "列表已整理"
    });
}

function queryDomains() {
    if (webStore.busy) return;
    const rawEntries = parseRawEntries(domainInput.value);
    if (rawEntries.length === 0) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请先输入需要查询的域名"
        });
        return;
    }

    for (const { value: item, line } of rawEntries) {
        if (!isPlainDomainInput(item)) {
            $q.notify({
                position: "top",
                type: "warning",
                message: `第 ${line} 行“${item.slice(0, 40)}”格式无效，请输入纯域名（如 example.com）`
            });
            return;
        }
    }

    const dedupedList = [...new Set(rawEntries.map((d) => d.value))];

    if (dedupedList.length > settingStore.webCheckDomainLimit) {
        $q.notify({
            position: "top",
            type: "warning",
            message: `去重后共 ${dedupedList.length} 个域名，单次最多查询 ${settingStore.webCheckDomainLimit} 个，请分批或使用“批量任务”`
        });
        return;
    }

    submitQuery("webCheck", { queryType: queryType.value, domains: dedupedList });
}

// Progress and the stop button describe this tab only; the store also holds Typo rows.
const pendingCount = computed(() => webCheckDomains.value.filter((row) => !row.isChecked).length);
const otherTabBusy = computed(() => webStore.busy && webStore.activeCheckType === "typoCheck");

const handleProgress = computed(() => {
    if (webCheckDomains.value.length === 0) return 0;
    return Math.round(((webCheckDomains.value.length - pendingCount.value) / webCheckDomains.value.length) * 100);
});
</script>
