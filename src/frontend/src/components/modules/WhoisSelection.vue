<template>
    <q-item class="q-py-none">
        <q-item-section side class="text-weight-bolder"> 查询接口 </q-item-section>
        <q-item-section>
            <div class="flex justify-left q-gutter-sm q-py-md">
                <q-radio
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    val="whoisQuery"
                    label="Whois"
                    :disable="disable"
                />
                <q-radio
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    val="whoisQueryWithProxy"
                    label="Whois + Proxy"
                    :disable="disable"
                />
                <q-radio
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    val="dnsQuery"
                    label="DNS"
                    :disable="disable"
                />
                <q-radio
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    val="mixedQuery"
                    label="Mixed"
                    :disable="disable"
                />

                <q-separator vertical class="q-ml-md" v-if="settingStore.whoisApis.length > 0 && $q.screen.gt.sm" />

                <q-radio
                    v-model="queryType"
                    checked-icon="check_circle"
                    unchecked-icon="radio_button_unchecked"
                    :val="api"
                    :label="api"
                    :disable="disable"
                    v-for="api in settingStore.whoisApis"
                    :key="api"
                />
            </div>
        </q-item-section>
    </q-item>
</template>

<script setup>
import { useSettingStore } from "src/stores/settingStore";

defineOptions({
    name: "WhoisSelection"
});

const props = defineProps({
    disable: {
        type: Boolean,
        default: false
    }
});

const queryType = defineModel("queryType");

const settingStore = useSettingStore();
</script>
