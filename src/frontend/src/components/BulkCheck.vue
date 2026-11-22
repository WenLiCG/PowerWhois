<template>
    <div class="flex justify-center q-gutter-md row q-py-lg">
        <q-spinner color="primary" size="3em" :thickness="8" v-if="loading" />
        <q-uploader
            :multiple="false"
            accept="text/plain"
            max-files="1"
            :max-file-size="100 * 1024 * 1024"
            auto-upload
            hide-upload-btn
            no-thumbnails
            flat
            bordered
            color="primary"
            :url="uploadUrl"
            :headers="[{ name: 'Authorization', value: 'Bearer ' + tokenStore.token }]"
            field-name="file"
            label="上传域名文件"
            class="col q-my-md"
            @failed="onUploadFailed"
            @rejected="onUploadRejected"
            v-if="showUploadBtn"
        ></q-uploader>

        <q-btn
            color="green"
            class="col"
            text-color="grey-9"
            icon="fa-regular fa-circle-play"
            label="开始任务"
            @click="startBulkCheckTask()"
            v-if="showStartBtn"
        ></q-btn>

        <q-btn
            color="blue"
            class="col"
            text-color="grey-9"
            icon="fa-regular fa-circle-pause"
            label="暂停任务"
            @click="pauseBulkCheckTask()"
            v-if="showPauseBtn"
        ></q-btn>

        <q-btn
            color="orange"
            class="col"
            text-color="grey-9"
            icon="fa-solid fa-forward-step"
            label="继续任务"
            @click="resumeBulkCheckTask()"
            v-if="showResumeBtn"
        ></q-btn>

        <q-btn
            color="negative"
            class="col"
            icon="fa-solid fa-delete-left"
            label="取消任务"
            @click="cancelBulkCheckTask()"
            v-if="showCancelBtn"
        ></q-btn>

        <q-btn
            color="green"
            class="col"
            text-color="grey-3"
            icon="fa-solid fa-square-plus"
            label="新建任务"
            @click="newBulkCheckTask()"
            v-if="showCreateBtn"
        ></q-btn>

        <q-btn
            color="blue"
            class="col"
            text-color="grey-3"
            icon="fa-solid fa-arrow-rotate-right"
            label="重查错误域名"
            @click="recheckErrorDomains()"
            v-if="showRequeryBtn"
        >
        </q-btn>

        <q-btn
            color="accent"
            class="col"
            icon="fa-solid fa-cloud-arrow-down"
            label="下载结果"
            @click="downloadBulkCheckTaskResult()"
            :loading="downloading"
            v-if="showDownloadBtn"
        >
            <template v-slot:loading>
                <q-spinner-hourglass class="on-left" />
                正在获取数据...
            </template>
        </q-btn>

        <q-spinner color="primary" size="3em" :thickness="8" v-if="showUniquingSpinner" />
    </div>

    <q-separator v-if="showQueryTypeSelection" />

    <div v-if="showQueryTypeSelection">
        <WhoisSelection v-model:queryType="queryType" />
    </div>

    <q-separator class="q-mb-md" v-if="showTaskStatus" />

    <div class="" v-if="showTaskStatus && bulkStore.bulkCheckQueryType">
        <span class="text-weight-bold q-mr-sm">当前查询类型:</span>
        <span class="text-weight-bold">
            <q-chip size="md">{{ bulkStore.bulkCheckQueryType }}</q-chip>
        </span>
    </div>
    <div class="q-mb-md" v-if="showTaskStatus">
        <span class="text-weight-bold q-mr-sm">当前任务状态:</span>
        <span class="text-weight-bold">
            <q-chip size="md" :color="taskStatusMsg.color" :text-color="taskStatusMsg.textColor">{{ taskStatusMsg.text }}</q-chip>
        </span>
    </div>

    <q-separator class="q-mb-md" v-if="showRuningProgress" />

    <q-linear-progress rounded size="25px" :value="bulkStore.runingProgress" color="accent" v-if="showRuningProgress">
        <div class="absolute-full flex flex-center">
            <q-badge color="white" text-color="accent" :label="bulkStore.runingProgressPercent" />
        </div>
    </q-linear-progress>

    <div class="q-pa-md q-gutter-sm" v-if="showRuningProgress">
        <q-tree :nodes="bulkStore.bulkCheckInfo" node-key="label" default-expand-all>
            <template v-slot:default-header="prop">
                <div class="row items-center text-subtitle1">
                    <q-icon :name="prop.node.icon" :color="prop.node.color" class="q-mr-sm" />
                    <div v-if="prop.node.id == 'root'">
                        <span class="">
                            <q-badge color="green" class="q-ml-sm">{{ prop.node.done }} (已完成)</q-badge>
                        </span>
                        <span class=""> / </span>
                        <span class="">
                            <q-badge color="primary">{{ prop.node.total }} (总共)</q-badge>
                        </span>
                    </div>
                    <div v-else>
                        <span class="">{{ prop.node.label }}: </span>
                        <q-badge :color="prop.node.color" class="q-ml-sm">{{ prop.node.value }}</q-badge>
                    </div>
                </div>
            </template>
        </q-tree>
    </div>
</template>

<script setup scoped>
defineOptions({
    name: "BulkCheck"
});

import { useQuasar } from "quasar";
import { ref, onMounted, watch } from "vue";
import { date } from "quasar";
import { api, apiUrl } from "boot/axios";
import { useTokenStore } from "src/stores/tokenStore";
import { useBulkStore } from "src/stores/bulkStore";
import { status, send } from "src/utils/websocketHandler";

import WhoisSelection from "src/components/modules/WhoisSelection.vue";

const $q = useQuasar();
const bulkStore = useBulkStore();
const tokenStore = useTokenStore();

const loading = ref(true);

const queryType = ref("whoisQuery");

const showUploadBtn = ref(false);
const showStartBtn = ref(false);
const showPauseBtn = ref(false);
const showResumeBtn = ref(false);
const showCancelBtn = ref(false);
const showCreateBtn = ref(false);
const showRequeryBtn = ref(false);
const showDownloadBtn = ref(false);
const showUniquingSpinner = ref(false);

const showQueryTypeSelection = ref(false);
const showRuningProgress = ref(false);
const showTaskStatus = ref(false);

const taskStatusMsg = ref({});

const downloading = ref(false);

function uploadUrl() {
    return `${apiUrl}/admin/bulkcheckupload`;
}

function onUploadFailed(info) {
    $q.notify({
        position: "top",
        type: "negative",
        message: info.xhr?.status === 409 ? "请先完成、暂停或清理当前任务再上传" : "上传文件失败，请检查文件大小并重试",
        progress: true,
        timeout: 10000,
        actions: [{ label: "确定", color: "white" }]
    });
}

function onUploadRejected(info) {
    $q.notify({
        position: "top",
        type: "negative",
        message: "选择的文件不符合要求",
        progress: true,
        timeout: 10000,
        actions: [{ label: "确定", color: "white" }]
    });
}

function isWebsocketConnected() {
    if (status.value == "OPEN") {
        return true;
    } else {
        $q.notify({
            position: "top",
            type: "warning",
            message: "网络连接失败, 请尝试刷新网页后重试",
            progress: true,
            timeout: 5000,
            actions: [{ label: "确定", color: "white" }]
        });
        return false;
    }
}

function startBulkCheckTask() {
    if (isWebsocketConnected()) {
        const bulkCheckStartData = {
            event: "bulkCheckStart",
            data: {
                queryType: queryType.value
            }
        };
        send(JSON.stringify(bulkCheckStartData));
    }
}

function pauseBulkCheckTask() {
    if (isWebsocketConnected()) {
        $q.dialog({
            title: "是否确认要暂停任务?",
            message: "暂停任务后可随时恢复任务",
            cancel: "取消",
            ok: "确定",
            persistent: true
        }).onOk(() => {
            const bulkCheckPauseData = {
                event: "bulkCheckPause",
                data: {}
            };
            send(JSON.stringify(bulkCheckPauseData));
        });
    }
}

function resumeBulkCheckTask() {
    if (isWebsocketConnected()) {
        const bulkCheckResumeData = {
            event: "bulkCheckResume",
            data: {}
        };
        send(JSON.stringify(bulkCheckResumeData));
    }
}

function cancelBulkCheckTask() {
    if (isWebsocketConnected()) {
        $q.dialog({
            title: "是否确认要取消任务?",
            message: "取消任务后将不可恢复, 请谨慎操作",
            cancel: "取消",
            ok: "确定",
            persistent: true
        }).onOk(() => {
            const bulkCheckCancelData = {
                event: "bulkCheckCancel",
                data: {}
            };
            send(JSON.stringify(bulkCheckCancelData));
        });
    }
}

function newBulkCheckTask() {
    if (isWebsocketConnected()) {
        $q.dialog({
            title: "是否确认要创建新任务?",
            message: "创建新任务将会清空当前任务数据，请在创建新任务前下载结果",
            cancel: "取消",
            ok: "确定",
            persistent: true
        }).onOk(() => {
            const bulkCheckClearMsg = {
                event: "bulkCheckClear",
                data: {}
            };
            send(JSON.stringify(bulkCheckClearMsg));
        });
    }
}

function recheckErrorDomains() {
    if (isWebsocketConnected()) {
        const bulkCheckRecheckMsg = {
            event: "bulkRecheckErrorDomains",
            data: {}
        };
        send(JSON.stringify(bulkCheckRecheckMsg));
    }
}

function downloadBulkCheckTaskResult() {
    const defaultFilename = "bulkCheckResult_" + date.formatDate(Date.now(), "YYYY-MM-DD_HH:mm:ss") + ".csv";

    downloading.value = true;

    api.get("/admin/bulkcheckresultdownload", {
        responseType: "blob",
        timeout: 600000
    })
        .then((response) => {
            let serverFilename = response.headers["content-disposition"]
                ? response.headers["content-disposition"].split("filename=")[1]
                : defaultFilename;

            // 去除可能存在的引号
            serverFilename = serverFilename.replace(/^["']|["']$/g, "");

            const url = window.URL.createObjectURL(new Blob([response.data]));
            const link = document.createElement("a");
            link.href = url;
            link.setAttribute("download", serverFilename);
            document.body.appendChild(link);
            link.click();

            // 清理并释放资源
            setTimeout(() => {
                document.body.removeChild(link);
                window.URL.revokeObjectURL(url);
            }, 100);

            downloading.value = false;
        })
        .catch((error) => {
            console.error("下载查询结果文件时发生错误:", error);
            $q.notify({
                position: "top",
                type: "negative",
                message: "下载查询结果文件失败, 请稍后重试"
            });

            downloading.value = false;
        });
}

watch(
    () => bulkStore.bulkCheckStatus,
    (newStatus) => {
        loading.value = false;
        switch (newStatus) {
            case "idle":
                showUploadBtn.value = true;
                showStartBtn.value = false;
                showPauseBtn.value = false;
                showResumeBtn.value = false;
                showCancelBtn.value = false;
                showCreateBtn.value = false;
                showRequeryBtn.value = false;
                showDownloadBtn.value = false;
                showUniquingSpinner.value = false;
                showQueryTypeSelection.value = false;
                showRuningProgress.value = false;
                showTaskStatus.value = false;
                taskStatusMsg.value = {
                    color: "info",
                    textColor: "white",
                    text: "空闲"
                };
                break;
            case "init":
                showUploadBtn.value = false;
                showStartBtn.value = true;
                showPauseBtn.value = false;
                showResumeBtn.value = false;
                showCancelBtn.value = true;
                showCreateBtn.value = false;
                showRequeryBtn.value = false;
                showDownloadBtn.value = false;
                showUniquingSpinner.value = false;
                showQueryTypeSelection.value = true;
                showRuningProgress.value = false;
                showTaskStatus.value = true;
                taskStatusMsg.value = {
                    color: "primary",
                    textColor: "white",
                    text: "已初始化"
                };
                break;
            case "uniquing":
                showUploadBtn.value = false;
                showStartBtn.value = false;
                showPauseBtn.value = false;
                showResumeBtn.value = false;
                showCancelBtn.value = false;
                showCreateBtn.value = false;
                showRequeryBtn.value = false;
                showDownloadBtn.value = false;
                showUniquingSpinner.value = true;
                showQueryTypeSelection.value = false;
                showRuningProgress.value = false;
                showTaskStatus.value = true;
                taskStatusMsg.value = {
                    color: "positive",
                    textColor: "white",
                    text: "统计和去重域名"
                };
                break;
            case "running":
                showUploadBtn.value = false;
                showStartBtn.value = false;
                showPauseBtn.value = true;
                showResumeBtn.value = false;
                showCancelBtn.value = true;
                showCreateBtn.value = false;
                showRequeryBtn.value = false;
                showDownloadBtn.value = false;
                showUniquingSpinner.value = false;
                showQueryTypeSelection.value = false;
                showRuningProgress.value = true;
                showTaskStatus.value = true;
                taskStatusMsg.value = {
                    color: "positive",
                    textColor: "white",
                    text: "运行中"
                };
                break;
            case "paused":
                showUploadBtn.value = false;
                showStartBtn.value = false;
                showPauseBtn.value = false;
                showResumeBtn.value = true;
                showCancelBtn.value = true;
                showCreateBtn.value = false;
                showRequeryBtn.value = false;
                showDownloadBtn.value = true;
                showUniquingSpinner.value = false;
                showQueryTypeSelection.value = false;
                showRuningProgress.value = true;
                showTaskStatus.value = true;
                taskStatusMsg.value = {
                    color: "accent",
                    textColor: "white",
                    text: "已暂停"
                };
                break;
            case "done":
                showUploadBtn.value = false;
                showStartBtn.value = false;
                showPauseBtn.value = false;
                showResumeBtn.value = false;
                showCancelBtn.value = false;
                showCreateBtn.value = true;
                showRequeryBtn.value = false;
                showDownloadBtn.value = true;
                showUniquingSpinner.value = false;
                showQueryTypeSelection.value = false;
                showRuningProgress.value = true;
                showTaskStatus.value = true;
                taskStatusMsg.value = {
                    color: "positive",
                    textColor: "white",
                    text: "已完成"
                };
                if (bulkStore.errorDomainsCount > 0) {
                    showRequeryBtn.value = true;
                }
                break;
            case "canceled":
                showUploadBtn.value = false;
                showStartBtn.value = false;
                showPauseBtn.value = false;
                showResumeBtn.value = false;
                showCancelBtn.value = false;
                showCreateBtn.value = true;
                showRequeryBtn.value = false;
                showDownloadBtn.value = true;
                showUniquingSpinner.value = false;
                showQueryTypeSelection.value = false;
                showRuningProgress.value = true;
                showTaskStatus.value = true;
                taskStatusMsg.value = {
                    color: "negative",
                    textColor: "white",
                    text: "已取消"
                };
                break;
            case "error":
                showUploadBtn.value = false;
                showStartBtn.value = false;
                showPauseBtn.value = false;
                showResumeBtn.value = bulkStore.canResume;
                showCancelBtn.value = false;
                showCreateBtn.value = true;
                showRequeryBtn.value = false;
                showDownloadBtn.value = true;
                showUniquingSpinner.value = false;
                showQueryTypeSelection.value = false;
                showRuningProgress.value = true;
                showTaskStatus.value = true;
                taskStatusMsg.value = {
                    color: "negative",
                    textColor: "white",
                    text: bulkStore.taskError || "任务出现错误"
                };
                break;
        }
    }
);

onMounted(() => {
    bulkStore.clearStatusAndInfo();
});
</script>
