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
                    :disable="busy"
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
                        :loading="typoCheckDomains.length > 0 && webStore.unCheckDomains > 0"
                        :percentage="handleProgress"
                        :disable="busy"
                        @click="typoCheckDomain()"
                    >
                        <template v-slot:loading>
                            <q-spinner-bars class="on-left" />
                            <span>{{ handleProgress }}%</span>
                        </template>
                    </q-btn>
                    <q-btn
                        v-if="webStore.unCheckDomains > 0"
                        outline
                        no-caps
                        color="negative"
                        icon="stop_circle"
                        label="停止"
                        style="height: 56px"
                        @click="stopTypoQuery()"
                    />
                </div>
            </div>
        </div>

        <q-separator class="q-mt-md" />

        <TypoSelection v-model:typoTypeSelected="typoTypeSelected" :disable="busy" />

        <q-separator />

        <CcTldSelection v-model:selectedTlds="typoSelectedCcTLDs" :disable="busy" />

        <q-separator />

        <WhoisSelection v-model:queryType="queryType" :disable="busy" />

        <template v-if="tokenStore.token && settingStore.registerApis.length">
            <q-separator />
            <RegisterSelection v-model:registerType="registerType" :disable="busy" />
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
import { status, send } from "src/utils/websocketHandler";
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
const busy = computed(() => webStore.unCheckDomains > 0 || webStore.unRegisterDomains > 0);
const settingStore = useSettingStore();

const domainInput = ref("");
const queryType = ref("whoisQuery");
const registerType = ref(null);

const resultType = ref("typoCheck");

watchEffect(() => {
    typoCheckDomains.value = webStore.domains.filter((domain) => domain.checkType == "typoCheck");
});

function isWebsocketConnected() {
    if (status.value == "OPEN") {
        return true;
    }
    $q.notify({
        position: "top",
        type: "warning",
        message: "服务连接已断开，请点击右上角“重连”或刷新页面后重试",
        progress: true,
        timeout: 5000,
        actions: [{ label: "确定", color: "white" }]
    });
    return false;
}

function typoCheckDomain() {
    if (busy.value) return;
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

    if (isWebsocketConnected()) {
        webStore.clearDomains();
        send(
            JSON.stringify({
                event: "typoCheck",
                data: {
                    domain: target,
                    typoType: typoTypeSelected.value,
                    ccTlds: typoSelectedCcTLDs.value,
                    queryType: queryType.value
                }
            })
        );
    }
}

function stopTypoQuery() {
    if (status.value === "OPEN") {
        send(JSON.stringify({ event: "webCheckStop", data: null }), false);
    }
    webStore.stopQueries();
    $q.notify({ position: "top", type: "info", message: "已停止当前 Typo 查询，已完成的结果已保留" });
}

const handleProgress = computed(() => {
    if (typoCheckDomains.value.length === 0) return 0;
    return Math.round(((typoCheckDomains.value.length - webStore.unCheckDomains) / typoCheckDomains.value.length) * 100);
});
</script>
