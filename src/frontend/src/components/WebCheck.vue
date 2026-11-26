<template>
    <q-card flat class="workspace-card q-pa-md">
        <div class="row items-center justify-between q-mb-sm q-gutter-xs">
            <div class="text-subtitle1 text-weight-bold text-grey-9">即时域名查询</div>
            <div class="row items-center q-gutter-xs">
                <q-badge
                    :color="uniqueInputCount > settingStore.webCheckDomainLimit ? 'negative' : 'blue-grey-7'"
                    outline
                    class="q-px-sm q-py-xs"
                >
                    已输入 {{ uniqueInputCount }} / {{ settingStore.webCheckDomainLimit }} 个域名
                </q-badge>
                <q-btn
                    flat
                    dense
                    no-caps
                    size="sm"
                    icon="auto_fix_high"
                    color="primary"
                    label="整理去重"
                    :disable="webStore.busy || !domainInput"
                    @click="cleanDomainInput"
                />
                <q-btn
                    flat
                    dense
                    no-caps
                    size="sm"
                    icon="clear_all"
                    color="grey-7"
                    label="清空"
                    :disable="webStore.busy || !domainInput"
                    @click="domainInput = ''"
                />
            </div>
        </div>

        <q-input
            outlined
            label="查询域名列表"
            hint="每行一个纯域名（如 example.com，支持中文 IDN 域名；不含 http:// 或路径）。快捷键：Ctrl + Enter 开始查询"
            class="full-width"
            v-model="domainInput"
            type="textarea"
            autogrow
            :input-style="{ minHeight: '110px', maxHeight: '280px' }"
            placeholder="example.com&#10;example.org&#10;例子.中国"
            :disable="webStore.busy"
            @keydown.ctrl.enter.prevent="queryDomains()"
            @keydown.meta.enter.prevent="queryDomains()"
        />

        <q-separator class="q-mt-md" />

        <WhoisSelection v-model:queryType="queryType" :disable="webStore.busy" />

        <template v-if="tokenStore.token && settingStore.registerApis.length">
            <q-separator />
            <RegisterSelection v-model:registerType="registerType" :disable="webStore.busy" />
        </template>

        <q-separator class="q-mb-md" />

        <div class="row q-col-gutter-sm items-center">
            <div class="col">
                <q-btn
                    unelevated
                    no-caps
                    color="primary"
                    icon="search"
                    label="开始查询"
                    class="full-width"
                    :loading="webStore.pendingQuery === 'webCheck' || pendingCount > 0"
                    :percentage="handleProgress"
                    :disable="webStore.busy"
                    @click="queryDomains()"
                >
                    <template v-slot:loading>
                        <q-spinner-bars class="on-left" />
                        <span v-if="webStore.pendingQuery === 'webCheck'">正在提交查询…</span>
                        <span v-else class="text-white">正在查询 {{ handleProgress }}%（剩余 {{ pendingCount }}）</span>
                    </template>
                </q-btn>
            </div>
            <div class="col-auto" v-if="pendingCount > 0">
                <q-btn
                    outline
                    no-caps
                    color="negative"
                    icon="stop_circle"
                    label="停止查询"
                    @click="stopQuery"
                    :loading="webStore.pendingQuery === 'stop'"
                />
            </div>
        </div>
        <div v-if="otherTabBusy" class="busy-elsewhere q-mt-sm" role="status">
            <q-icon name="hourglass_top" size="16px" />
            Typo 页面有查询或注册正在进行；完成或在 Typo 页面停止后可开始新的查询，本页结果会保留。
        </div>
    </q-card>

    <div v-if="webCheckDomains.length > 0" class="q-mt-md">
        <CheckedResult :domainResults="webCheckDomains" :resultType="resultType" :queryType="queryType" :registerType="registerType" />
    </div>
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

<style scoped>
.busy-elsewhere {
    display: flex;
    align-items: center;
    gap: 6px;
    color: #8a5a00;
    font-size: 13px;
}
</style>
