<template>
    <q-item class="q-py-sm q-px-none">
        <q-item-section side class="text-weight-bold text-grey-9" style="min-width: 84px">
            <div>Typo 类型</div>
            <div class="row q-gutter-xs q-mt-xs">
                <q-btn flat dense no-caps size="xs" color="primary" label="全选" :disable="disable" style="min-height: 22px" @click="selectAll" />
                <q-btn flat dense no-caps size="xs" color="grey-7" label="清空" :disable="disable" style="min-height: 22px" @click="typoTypeSelected = []" />
            </div>
        </q-item-section>
        <q-item-section>
            <div class="flex justify-left q-gutter-sm">
                <q-checkbox
                    :disable="disable"
                    v-model="typoTypeSelected"
                    :val="typoType"
                    :label="typoLabel"
                    color="primary"
                    v-for="(typoLabel, typoType) in typoTypes"
                    :key="typoType"
                />
            </div>
        </q-item-section>
    </q-item>
</template>

<script setup>
import { onMounted } from "vue";
import { typoTypes } from "src/utils/globalDefinition";

defineOptions({
    name: "TypoSelection"
});

defineProps({
    disable: {
        type: Boolean,
        default: false
    }
});

const typoTypeSelected = defineModel("typoTypeSelected", {
    default: () => ["www", "skipLetter", "wrongHorizontalKey"]
});

function selectAll() {
    typoTypeSelected.value = Object.keys(typoTypes);
}

onMounted(() => {
    if (typoTypeSelected.value.length === 0) {
        typoTypeSelected.value = ["www", "skipLetter", "wrongHorizontalKey"];
    }
});
</script>
