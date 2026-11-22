<template>
    <div class="q-py-md">
        <div class="row items-center q-mb-md">
            <div class="text-subtitle1 text-weight-bold">待核对的注册操作</div>
            <q-space />
            <q-btn flat color="primary" icon="refresh" label="刷新" :loading="loading" @click="load" />
        </div>
        <div class="text-caption text-grey-8 q-mb-md">
            这些操作的结果不确定，对应域名在核对前不会再次提交。请先在注册商后台确认订单结果，再记录为成功或失败。
        </div>
        <q-table
            :rows="rows"
            :columns="columns"
            row-key="operationId"
            :loading="loading"
            :pagination="{ rowsPerPage: 50 }"
            no-data-label="没有待核对的注册操作"
            class="no-shadow"
            bordered
        >
            <template v-slot:body-cell-registerStatus="props">
                <q-td :props="props">
                    <q-chip dense color="orange" text-color="white">
                        {{ props.row.registerStatus === "submitting" ? "提交中或中断" : "待核对" }}
                        <q-tooltip>{{ props.row.rawResponse }}</q-tooltip>
                    </q-chip>
                </q-td>
            </template>
            <template v-slot:body-cell-actions="props">
                <q-td :props="props">
                    <q-btn flat dense label="已核实失败" :disable="busy" @click="reconcile(props.row, 'failed')" />
                    <q-btn flat dense color="primary" label="已核实成功" :disable="busy" @click="reconcile(props.row, 'success')" />
                </q-td>
            </template>
        </q-table>
    </div>
</template>

<script setup>
import { ref, onMounted } from "vue";
import { useQuasar } from "quasar";
import { api } from "boot/axios";

defineOptions({
    name: "PendingRegistrations"
});

const $q = useQuasar();
const rows = ref([]);
const loading = ref(false);
const busy = ref(false);

const columns = [
    { name: "domainName", label: "域名", field: "domainName", align: "left", sortable: true },
    { name: "registerType", label: "注册接口", field: "registerType", align: "left", sortable: true },
    {
        name: "startedAt",
        label: "开始时间",
        field: "startedAt",
        align: "left",
        sortable: true,
        format: (value) => (value ? new Date(value * 1000).toLocaleString() : "")
    },
    { name: "registerStatus", label: "状态", field: "registerStatus", align: "center" },
    { name: "operationId", label: "操作编号", field: "operationId", align: "left" },
    { name: "actions", label: "", field: "operationId", align: "right" }
];

function load() {
    loading.value = true;
    api.get("/admin/registrations")
        .then((response) => {
            rows.value = Array.isArray(response.data) ? response.data : [];
        })
        .catch(() => {
            $q.notify({ position: "top", type: "negative", message: "无法读取待核对的注册操作" });
        })
        .finally(() => {
            loading.value = false;
        });
}

function reconcile(row, outcome) {
    if (busy.value) return;
    $q.dialog({
        title: "保存人工核对结果",
        message: "请先在注册商确认 " + row.domainName + " 的订单结果，再记录为" + (outcome === "success" ? "成功" : "失败") + "。",
        cancel: true,
        persistent: true
    }).onOk(async () => {
        busy.value = true;
        try {
            await api.put("/admin/registration/" + row.operationId, { domain: row.domainName, outcome });
            rows.value = rows.value.filter((item) => item.operationId !== row.operationId);
            $q.notify({ position: "top", type: "positive", message: "已保存核对结果" });
        } catch {
            $q.notify({ position: "top", type: "negative", message: "无法保存：操作可能仍在提交（30 秒内），或已被处理" });
            load();
        } finally {
            busy.value = false;
        }
    });
}

onMounted(load);
</script>
