<template>
    <q-item class="q-py-sm q-px-none">
        <q-item-section side class="text-weight-bold text-grey-9" style="min-width: 84px">
            <div>后缀变体</div>
            <div class="row q-gutter-xs q-mt-xs">
                <q-btn flat dense no-caps size="xs" color="primary" label="全选" :disable="disable" style="min-height: 22px" @click="selectedTlds = [...tldOptions]" />
                <q-btn flat dense no-caps size="xs" color="grey-7" label="清空" :disable="disable" style="min-height: 22px" @click="selectedTlds = []" />
            </div>
        </q-item-section>
        <q-item-section>
            <div class="flex justify-left items-center q-gutter-sm">
                <q-checkbox :disable="disable" v-model="selectedTlds" :val="tld" :label="tld" color="primary" v-for="tld in tldOptions" :key="tld" />
                <q-input
                    dense
                    outlined
                    :disable="disable"
                    v-model="inputTld"
                    label="添加后缀"
                    placeholder="例如 io 或 co.uk"
                    style="width: 190px"
                    @keydown.enter.prevent="addTld"
                >
                    <template v-slot:after>
                        <q-btn aria-label="添加后缀" dense unelevated icon="add" color="primary" :disable="disable || !inputTld" @click="addTld" />
                    </template>
                </q-input>
            </div>
        </q-item-section>
    </q-item>
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
