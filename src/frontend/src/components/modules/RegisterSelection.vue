<template>
    <q-expansion-item icon="shopping_cart" label="手动注册接口" class="suffix-options">
        <div class="mode-options">
            <q-radio v-for="api in settingStore.registerApis" :key="api" v-model="registerType" :val="api" :label="api" color="primary" :disable="disable" />
            <q-btn v-if="registerType" flat dense no-caps label="取消选择" :disable="disable" @click="registerType = null" />
        </div>
        <p v-if="registerType" class="mode-description">已选 {{ registerType }}。勾选查询结果并确认后才会提交注册。</p>
    </q-expansion-item>
</template>

<script setup>
import { watch } from "vue";
import { useSettingStore } from "src/stores/settingStore";

defineOptions({
    name: "RegisterSelection"
});

defineProps({
    disable: {
        type: Boolean,
        default: false
    }
});

const registerType = defineModel("registerType");

const settingStore = useSettingStore();

watch(
    () => settingStore.registerApis,
    (apis) => {
        if (registerType.value && !apis.includes(registerType.value)) {
            registerType.value = null;
        }
    }
);
</script>
