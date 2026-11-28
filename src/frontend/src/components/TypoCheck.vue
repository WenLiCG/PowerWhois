<template>
    <header class="page-heading"><h1>Typo 变体查询</h1><p>从一个域名生成拼写或后缀变体，并查询各候选域名的注册状态。</p></header>
    <q-card flat class="workspace-card lookup-card">
        <div class="lookup-grid">
            <section class="domain-editor typo-editor">
                <label class="section-label" for="typo-domain">原始域名</label>
                <q-input id="typo-domain" outlined clearable v-model="domainInput" aria-label="原始主域名" placeholder="example.com"
                    autocapitalize="none" autocomplete="off" spellcheck="false" @keyup.enter="typoCheckDomain()" :disable="webStore.busy" />
                <TypoSelection v-model:typoTypeSelected="typoTypeSelected" :disable="webStore.busy" />
                <q-expansion-item icon="public" class="suffix-options" :label="`后缀变体（已选 ${typoSelectedCcTLDs.length}）`">
                    <CcTldSelection v-model:selectedTlds="typoSelectedCcTLDs" :disable="webStore.busy" />
                </q-expansion-item>
            </section>
            <aside class="query-sidebar">
                <WhoisSelection v-model:queryType="queryType" :disable="webStore.busy" />
                <RegisterSelection v-if="tokenStore.token && settingStore.registerApis.length" v-model:registerType="registerType" :disable="webStore.busy" />
            </aside>
        </div>
        <div class="form-action-bar">
            <span class="action-summary">已选 {{ typoTypeSelected.length }} 种拼写规则、{{ typoSelectedCcTLDs.length }} 个后缀</span>
            <div class="form-buttons">
                <q-btn v-if="pendingCount > 0" outline no-caps color="negative" icon="stop_circle" label="停止" @click="stopQuery"
                    :loading="webStore.pendingQuery === 'stop'" />
                <q-btn unelevated no-caps color="primary" icon="spellcheck" label="生成并查询" class="primary-action" @click="typoCheckDomain()"
                    :loading="webStore.pendingQuery === 'typoCheck' || pendingCount > 0" :percentage="handleProgress" :disable="webStore.busy">
                    <template #loading><q-spinner class="on-left" />{{ webStore.pendingQuery === "typoCheck" ? "正在生成…" : `查询中 ${handleProgress}%` }}</template>
                </q-btn>
            </div>
        </div>
        <p v-if="otherTabBusy" class="busy-message" role="status">网页查询正在进行。完成后即可查询新的变体。</p>
    </q-card>
    <CheckedResult v-if="typoCheckDomains.length" :domainResults="typoCheckDomains" :resultType="resultType" :queryType="queryType" :registerType="registerType" />
</template>

<script setup>
defineOptions({
    name: "TypoCheck"
});

import { ref, computed, watchEffect } from "vue";
import { useQuasar } from "quasar";
import { submitQuery, stopQuery } from "src/utils/websocketHandler";
import { isPlainDomainInput, normalizeDomainInput } from "src/utils/domainInput";
import { useTokenStore } from "src/stores/tokenStore";
import { useWebStore } from "src/stores/webStore";
import { useSettingStore } from "src/stores/settingStore";

import TypoSelection from "src/components/modules/TypoSelection.vue";
import CcTldSelection from "src/components/modules/CcTldSelection.vue";
import WhoisSelection from "src/components/modules/WhoisSelection.vue";
import RegisterSelection from "src/components/modules/RegisterSelection.vue";
import CheckedResult from "src/components/modules/CheckedResult.vue";

const $q = useQuasar();

const typoTypeSelected = ref([]);
const typoSelectedCcTLDs = ref([]);

const typoCheckDomains = ref([]);

const tokenStore = useTokenStore();
const webStore = useWebStore();

const settingStore = useSettingStore();

const domainInput = computed({
    get: () => webStore.typoDomainInput,
    set: (val) => {
        webStore.typoDomainInput = val || "";
    }
});
const queryType = computed({
    get: () => webStore.typoQueryType,
    set: (val) => {
        webStore.typoQueryType = val;
    }
});
const registerType = ref(null);

const resultType = ref("typoCheck");

watchEffect(() => {
    typoCheckDomains.value = webStore.domains.filter((domain) => domain.checkType == "typoCheck");
});

function typoCheckDomain() {
    if (webStore.busy) return;
    const target = normalizeDomainInput(domainInput.value);
    if (!target) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请输入原始域名"
        });
        return;
    }
    if (!isPlainDomainInput(target)) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请输入合法的纯域名（如 example.com），不含 http:// 或路径"
        });
        return;
    }
    if (typoTypeSelected.value.length == 0 && typoSelectedCcTLDs.value.length == 0) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请至少选择一种 Typo 类型或后缀变体（ccTLDs）"
        });
        return;
    }
    if (!queryType.value) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请选择查询接口"
        });
        return;
    }

    submitQuery("typoCheck", {
        domain: target, typoType: typoTypeSelected.value, ccTlds: typoSelectedCcTLDs.value, queryType: queryType.value
    });
}

// Progress and the stop button describe this tab only; the store also holds instant-lookup rows.
const pendingCount = computed(() => typoCheckDomains.value.filter((row) => !row.isChecked).length);
const otherTabBusy = computed(() => webStore.busy && webStore.activeCheckType === "webCheck");

const handleProgress = computed(() => {
    if (typoCheckDomains.value.length === 0) return 0;
    return Math.round(((typoCheckDomains.value.length - pendingCount.value) / typoCheckDomains.value.length) * 100);
});
</script>
