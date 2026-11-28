<template>
    <fieldset class="mode-picker">
        <legend>查询方式</legend>
        <div class="mode-options">
            <label v-for="option in options" :key="option.value" class="mode-option" :class="{ 'is-selected': queryType === option.value }">
                <input type="radio" :name="groupId" v-model="queryType" :value="option.value" :disabled="disable" :aria-label="option.label" />
                <span>{{ option.label }}</span>
            </label>
        </div>
        <p class="mode-description">{{ options.find(option => option.value === queryType)?.description }}</p>
        <p v-if="tokenStore.token && queryType === 'whoisQueryWithProxy' && !settingStore.proxyConfigured" class="proxy-hint" role="status">
            请先在系统设置中填写 SOCKS5 代理并测试连接。
        </p>
    </fieldset>
</template>

<script setup>
import { computed, useId, watch } from "vue";
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
const groupId = useId();
const options = computed(() => [
    { value: "whoisQuery", label: "Whois (RDAP 优先)", description: "优先读取注册局 RDAP 记录，其余支持的后缀使用 WHOIS。" },
    { value: "dnsQuery", label: "DNS", description: "向权威服务器查询 NS 委派。无委派记录时，注册状态为未知。" },
    { value: "mixedQuery", label: "Mixed 混合", description: "按已配置的后缀策略选择 DNS、直连或代理查询。" },
    ...(tokenStore.token ? [
        { value: "whoisQueryWithProxy", label: "Whois + 代理", description: "通过已配置的 SOCKS5 代理读取注册局记录。" },
        ...settingStore.whoisApis.map((name) => ({ value: name, label: `自定义: ${name}`, description: "使用管理员配置的查询接口与结果匹配规则。" }))
    ] : [])
]);

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
    },
    { immediate: true }
);
</script>
