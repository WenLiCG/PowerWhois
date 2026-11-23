<template>
    <div class="flex justify-center">
        <q-input outlined autofocus class="full-width" v-model="domainInput" label="原始域名" placeholder="example.com" @keyup.enter="typoCheckDomain()" :disable="webStore.unCheckDomains > 0">
            <template v-slot:append>
                <q-btn icon="send" color="primary" label="生成并查询" :disable="webStore.unCheckDomains > 0" @click="typoCheckDomain()" />
            </template>
        </q-input>
    </div>

    <q-separator class="q-mt-md" />

    <div>
        <TypoSelection v-model:typoTypeSelected="typoTypeSelected" :disable="webStore.unCheckDomains > 0" />
    </div>

    <q-separator />

    <div>
        <CcTldSelection v-model:selectedTlds="typoSelectedCcTLDs" :disable="webStore.unCheckDomains > 0" />
    </div>

    <q-separator />

    <div>
        <WhoisSelection v-model:queryType="queryType" :disable="webStore.unCheckDomains > 0" />
    </div>

    <q-separator v-if="tokenStore.token" />

    <div v-if="tokenStore.token">
        <RegisterSelection v-model:registerType="registerType" :disable="webStore.unCheckDomains > 0" />
    </div>

    <q-separator class="q-my-md" />

    <div v-if="typoCheckDomains.length > 0">
        <CheckedResult :domainResults="typoCheckDomains" :resultType="resultType" :queryType="queryType" :registerType="registerType" />
    </div>
</template>

<script setup scoped>
defineOptions({
    name: "TypoCheck"
});

import { ref, computed, onMounted, watch, watchEffect } from "vue";
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
const settingStore = useSettingStore();

const domainInput = ref(null);
const queryType = ref("whoisQuery");
const registerType = ref(null);

const resultType = ref("typoCheck");

watchEffect(() => {
    typoCheckDomains.value = webStore.domains.filter((domain) => domain.checkType == "typoCheck");
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

function typoCheckDomain() {
    if (domainInput.value) {
        if (typoTypeSelected.value.length == 0 && typoSelectedCcTLDs.value.length == 0) {
            $q.notify({
                position: "top",
                type: "warning",
                message: "请选择Typo类型或ccTLDs"
            });
            return;
        }
        if (!queryType.value) {
            $q.notify({
                position: "top",
                type: "warning",
                message: "请选择查询类型"
            });
            return;
        }

        if (isWebsocketConnected()) {
            webStore.clearDomains();
            const typoCheckData = {
                event: "typoCheck",
                data: {
                    domain: domainInput.value,
                    typoType: typoTypeSelected.value,
                    ccTlds: typoSelectedCcTLDs.value,
                    queryType: queryType.value
                }
            };
            send(JSON.stringify(typoCheckData));
        }
    } else {
        $q.notify({
            position: "top",
            type: "warning",
            message: "请输入域名"
        });
        return;
    }
}

const handleProgress = computed(() => {
    if (typoCheckDomains.value.length === 0) return 0;
    return Math.round(((typoCheckDomains.value.length - webStore.unCheckDomains) / typoCheckDomains.value.length) * 100);
});

onMounted(() => {});
</script>
