<template>
    <q-item class="q-py-sm q-px-none">
        <q-item-section side class="text-weight-bold text-grey-9" style="min-width: 84px">查询接口</q-item-section>
        <q-item-section>
            <div class="flex justify-left items-center q-gutter-sm">
                <q-radio
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    val="whoisQuery"
                    label="Whois (RDAP 优先)"
                    :disable="disable"
                >
                    <q-tooltip>优先使用 HTTPS RDAP 查询，不支持 RDAP 的后缀自动使用传统 WHOIS</q-tooltip>
                </q-radio>
                <q-radio
                    v-if="tokenStore.token"
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    val="whoisQueryWithProxy"
                    label="Whois + 代理"
                    :disable="disable"
                >
                    <q-tooltip>通过系统设置中配置的 SOCKS5 代理执行 RDAP / WHOIS 查询</q-tooltip>
                </q-radio>
                <q-radio
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    val="dnsQuery"
                    label="DNS"
                    :disable="disable"
                >
                    <q-tooltip>权威 NS 委派查询：有 NS 判为已注册，无 NS 标记为未知（不等于未注册）</q-tooltip>
                </q-radio>
                <q-radio
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    val="mixedQuery"
                    label="Mixed 混合"
                    :disable="disable"
                >
                    <q-tooltip>按后缀策略自动分流 DNS、直连 WHOIS/RDAP 或代理查询</q-tooltip>
                </q-radio>

                <template v-if="tokenStore.token && settingStore.whoisApis.length > 0">
                    <q-separator vertical inset class="q-mx-sm" v-if="$q.screen.gt.sm" />

                    <q-radio
                        v-model="queryType"
                        checked-icon="check_circle"
                        unchecked-icon="radio_button_unchecked"
                        :val="api"
                        :label="`自定义: ${api}`"
                        :disable="disable"
                        v-for="api in settingStore.whoisApis"
                        :key="api"
                    />
                </template>
            </div>
            <div v-if="tokenStore.token && queryType === 'whoisQueryWithProxy' && !settingStore.proxyConfigured" class="proxy-hint q-mt-xs" role="status">
                <q-icon name="info_outline" size="15px" />
                <span>尚未检测到已保存的 SOCKS5 代理，请先前往右上角「系统设置」填写代理主机与端口并测试连接。</span>
            </div>
        </q-item-section>
    </q-item>
</template>

<script setup>
import { watch } from "vue";
import { useSettingStore } from "src/stores/settingStore";
import { useTokenStore } from "src/stores/tokenStore";

defineOptions({
    name: "WhoisSelection"
});

defineProps({
    disable: {
        type: Boolean,
        default: false
    }
});

const queryType = defineModel("queryType");

const settingStore = useSettingStore();
const tokenStore = useTokenStore();

watch(
    () => [tokenStore.token, settingStore.whoisApis],
    ([token, apis]) => {
        if (!token && (queryType.value === "whoisQueryWithProxy" || apis.includes(queryType.value))) {
            queryType.value = "whoisQuery";
        } else if (
            token &&
            !["whoisQuery", "whoisQueryWithProxy", "dnsQuery", "mixedQuery"].includes(queryType.value) &&
            !apis.includes(queryType.value)
        ) {
            queryType.value = "whoisQuery";
        }
    }
);
</script>
