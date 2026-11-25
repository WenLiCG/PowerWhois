<template>
    <q-item class="q-py-sm q-px-none">
        <q-item-section side class="text-weight-bold text-grey-9" style="min-width: 84px">注册接口</q-item-section>
        <q-item-section>
            <div class="flex justify-left items-center q-gutter-sm">
                <q-radio
                    v-model="registerType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    :val="api"
                    :label="api"
                    :disable="disable"
                    v-for="api in settingStore.registerApis"
                    :key="api"
                />
                <q-btn
                    v-if="registerType"
                    flat
                    dense
                    no-caps
                    size="xs"
                    color="grey-7"
                    label="取消选择"
                    :disable="disable"
                    @click="registerType = null"
                />
            </div>
        </q-item-section>
    </q-item>
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
