<template>
    <q-card flat class="workspace-card q-pa-md">
        <div class="text-subtitle1 text-weight-bold text-grey-9 q-mb-sm">Typo 变体生成与查询</div>
        <div class="row q-col-gutter-sm items-start">
            <div class="col">
                <q-input
                    outlined
                    autofocus
                    clearable
                    class="full-width"
                    v-model="domainInput"
                    label="原始主域名"
                    hint="输入单个基准域名（如 example.com），自动生成拼写变体并查询注册状态"
                    placeholder="example.com"
                    @keyup.enter="typoCheckDomain()"
                    :disable="webStore.busy"
                />
            </div>
            <div class="col-12 col-sm-auto">
                <div class="row q-gutter-sm no-wrap">
                    <q-btn
                        unelevated
                        no-caps
                        icon="send"
                        color="primary"
                        label="生成并查询"
                        style="height: 56px"
                        class="q-px-lg full-width"
                        :loading="webStore.pendingQuery === 'typoCheck' || pendingCount > 0"
                        :percentage="handleProgress"
                        :disable="webStore.busy"
                        @click="typoCheckDomain()"
                    >
                        <template v-slot:loading>
                            <q-spinner-bars class="on-left" />
                            <span v-if="webStore.pendingQuery === 'typoCheck'">正在提交变体查询…</span>
                            <span v-else>{{ handleProgress }}%</span>
                        </template>
                    </q-btn>
                    <q-btn
                        v-if="pendingCount > 0"
                        outline
                        no-caps
                        color="negative"
                        icon="stop_circle"
                        label="停止"
                        style="height: 56px"
                        @click="stopQuery"
                        :loading="webStore.pendingQuery === 'stop'"
                    />
                </div>
            </div>
        </div>
        <div v-if="otherTabBusy" class="busy-elsewhere q-mt-sm" role="status">
            <q-icon name="hourglass_top" size="16px" />
            网页查询页面有查询或注册正在进行；完成或在该页面停止后可开始新的变体查询，本页结果会保留。
        </div>

        <q-separator class="q-mt-md" />

        <TypoSelection v-model:typoTypeSelected="typoTypeSelected" :disable="webStore.busy" />

        <q-separator />

        <CcTldSelection v-model:selectedTlds="typoSelectedCcTLDs" :disable="webStore.busy" />

        <q-separator />

        <WhoisSelection v-model:queryType="queryType" :disable="webStore.busy" />

        <template v-if="tokenStore.token && settingStore.registerApis.length">
            <q-separator />
            <RegisterSelection v-model:registerType="registerType" :disable="webStore.busy" />
        </template>
    </q-card>

    <div v-if="typoCheckDomains.length > 0" class="q-mt-md">
        <CheckedResult :domainResults="typoCheckDomains" :resultType="resultType" :queryType="queryType" :registerType="registerType" />
    </div>
</template>

<script setup>
defineOptions({
    name: "TypoCheck"
});

import { ref, computed, watchEffect } from "vue";
import { useQuasar } from "quasar";
import { submitQuery, stopQuery } from "src/utils/websocketHandler";
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
    const target = (domainInput.value || "").trim().replace(/\.+$/, "");
    if (!target) {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请输入原始域名"
        });
        return;
    }
    if (target.includes("://") || target.includes("/") || /\s/.test(target) || !target.includes(".")) {
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

<style scoped>
.busy-elsewhere {
    display: flex;
    align-items: center;
    gap: 6px;
    color: #8a5a00;
    font-size: 13px;
}
</style>
