<template>
    <div class="flex justify-center">
        <q-input
            outlined
            label="查询域名"
            hint="每行一个纯域名，例如 example.com；不填写网址或路径"
            clearable
            class="full-width"
            v-model="domainInput"
            type="textarea"
            placeholder="请输入查询域名, 每行一个域名"
            :disable="webStore.unCheckDomains > 0"
        />
    </div>

    <q-separator class="q-mt-md" />

    <div>
        <WhoisSelection v-model:queryType="queryType" :disable="webStore.unCheckDomains > 0" />
    </div>

    <q-separator v-if="tokenStore.token" />

    <div v-if="tokenStore.token">
        <RegisterSelection v-model:registerType="registerType" :disable="webStore.unCheckDomains > 0" />
    </div>

    <q-separator class="q-mb-md" />

    <div class="flex justify-center">
        <q-btn
            unelevated
            color="primary"
            label="开始查询"
            class="full-width"
            :loading="webCheckDomains.length > 0 && webStore.unCheckDomains > 0"
            :percentage="handleProgress"
            :disable="webStore.unCheckDomains > 0"
            @click="queryDomains()"
        >
            <template v-slot:loading>
                <q-spinner-bars class="on-left" />
                <span class="text-white">{{ handleProgress }}%</span>
            </template>
        </q-btn>
    </div>

    <q-separator class="q-my-md" />

    <div v-if="webCheckDomains.length > 0">
        <CheckedResult :domainResults="webCheckDomains" :resultType="resultType" :queryType="queryType" :registerType="registerType" />
    </div>
</template>

<script setup scoped>
defineOptions({
    name: "WebCheck"
});

import { ref, computed, onMounted, watch, watchEffect } from "vue";
import { useQuasar } from "quasar";
import { status, send } from "src/utils/websocketHandler";
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

const domainInput = ref(null);
const queryType = ref("whoisQuery");
const registerType = ref(null);

const resultType = ref("webCheck");

watchEffect(() => {
    webCheckDomains.value = webStore.domains.filter((domain) => domain.checkType == "webCheck");
});

function isWebsocketConnected() {
    if (status.value == "OPEN") {
        return true;
    } else {
        $q.notify({
            position: "top",
            type: "warning",
            message: "网络连接失败, 请尝试刷新网页后重试",
            progress: true,
            timeout: 5000,
            actions: [{ label: "确定", color: "white" }]
        });
        return false;
    }
}

function queryDomains() {
    if (domainInput.value) {
        if (isWebsocketConnected()) {
            let inputDomainList = domainInput.value.split("\n").filter((line) => line.trim() !== "");
            if (inputDomainList.length > settingStore.webCheckDomainLimit) {
                $q.notify({
                    position: "top",
                    type: "warning",
                    message: "每次最多查询 " + settingStore.webCheckDomainLimit + " 个域名, 请分批次输入"
                });
            } else {
                webStore.clearDomains();

                const webCheckData = {
                    event: "webCheck",
                    data: {
                        queryType: queryType.value,
                        domains: inputDomainList
                    }
                };
                send(JSON.stringify(webCheckData));
            }
        }
    } else {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请先输入需要查询的域名"
        });
    }
}

const handleProgress = computed(() => {
    if (webCheckDomains.value.length === 0) return 0;
    return Math.round(((webCheckDomains.value.length - webStore.unCheckDomains) / webCheckDomains.value.length) * 100);
});

onMounted(() => {});
</script>
