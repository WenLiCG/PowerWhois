<template>
    <fieldset class="choice-fieldset">
        <legend>拼写规则</legend>
        <div class="choice-toolbar">
            <q-btn flat dense no-caps label="全选" aria-label="全选拼写规则" :disable="disable" @click="selectAll" />
            <q-btn flat dense no-caps label="清空" aria-label="清空拼写规则" :disable="disable" @click="typoTypeSelected = []" />
        </div>
        <div class="choice-options">
            <q-checkbox v-for="(label, type) in typoTypes" :key="type" v-model="typoTypeSelected" :val="type" :label="label"
                dense color="primary" :disable="disable" />
        </div>
    </fieldset>
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
