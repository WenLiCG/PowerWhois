<template>
    <div class="suffix-editor">
        <div class="input-tools">
            <q-btn flat dense no-caps label="全选" aria-label="全选后缀" :disable="disable" @click="selectedTlds = [...tldOptions]" />
            <q-btn flat dense no-caps label="清空" aria-label="清空后缀" :disable="disable" @click="selectedTlds = []" />
        </div>
        <div class="choice-options">
            <q-checkbox v-for="tld in tldOptions" :key="tld" v-model="selectedTlds" :val="tld" :label="tld" dense color="primary" :disable="disable" />
        </div>
        <q-input dense outlined :disable="disable" v-model="inputTld" label="添加后缀" placeholder="例如 io 或 co.uk" @keydown.enter.prevent="addTld">
            <template #append><q-btn aria-label="添加后缀" flat dense round icon="add" color="primary" :disable="disable || !inputTld" @click="addTld" /></template>
        </q-input>
    </div>
</template>

<script setup>
import { ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useSettingStore } from "src/stores/settingStore";

defineOptions({
    name: "CcTldSelection"
});

defineProps({
    disable: {
        type: Boolean,
        default: false
    }
});

const selectedTlds = defineModel("selectedTlds");

const $q = useQuasar();
const settingStore = useSettingStore();

const tldOptions = ref([]);
const inputTld = ref("");

function addTld() {
    if (inputTld.value) {
        const newTld = inputTld.value.replace(/\s+/g, "").replace(/^\.+/, "").replace(/\.+$/, "").toLowerCase();
        if (newTld.length > 0) {
            if (tldOptions.value.includes(newTld)) {
                if (!selectedTlds.value.includes(newTld)) {
                    selectedTlds.value = [...selectedTlds.value, newTld];
                }
                inputTld.value = "";
                $q.notify({
                    position: "top",
                    type: "info",
                    message: `后缀 .${newTld} 已勾选`
                });
            } else {
                tldOptions.value = [...tldOptions.value, newTld];
                selectedTlds.value = [...selectedTlds.value, newTld];
                inputTld.value = "";
            }
        } else {
            $q.notify({
                position: "top",
                type: "warning",
                message: "输入的后缀无效"
            });
        }
    }
}

watch(
    () => settingStore.typoDefaultCcTlds,
    () => {
        selectedTlds.value = [];
        tldOptions.value = [];
        settingStore.typoDefaultCcTlds.forEach((tld) => {
            if (tld.isSelected) {
                selectedTlds.value.push(tld.tld);
            }
            tldOptions.value.push(tld.tld);
        });
    },
    { immediate: true }
);
</script>
