<template>
    <q-layout class="bg-grey-1">
        <q-page-container>
            <q-page class="">
                <div class="full-width">
                    <q-tabs
                        v-model="tab"
                        align="center"
                        inline-label
                        no-caps
                        class="bg-light-blue-1"
                        active-bg-color="light-blue-2"
                        active-color="primary"
                        indicator-color="positive"
                    >
                        <q-tab name="webCheck" icon="search" label="网页查询" />
                        <q-separator vertical />
                        <q-tab name="bulkCheck" icon="zoom_in" label="批量查询" v-if="tokenStore.token" />
                        <q-separator vertical v-if="tokenStore.token" />
                        <q-tab name="typoCheck" icon="text_format" label="Typo查询" v-if="tokenStore.token" />
                        <q-separator vertical v-if="tokenStore.token" />
                        <q-tab name="login" icon="person" label="登录" v-if="!tokenStore.token" />
                        <q-btn-dropdown auto-close stretch flat icon="manage_accounts" label="管理" v-if="tokenStore.token">
                            <q-list>
                                <q-item clickable @click="tab = 'setting'">
                                    <q-item-section>设置</q-item-section>
                                </q-item>
                                <q-item clickable @click="logout">
                                    <q-item-section>退出登录</q-item-section>
                                </q-item>
                            </q-list>
                        </q-btn-dropdown>
                    </q-tabs>
                </div>
                <div style="max-width: 1200px; margin: 0 auto">
                    <q-tab-panels v-model="tab" animated class="full-width bg-grey-1">
                        <q-tab-panel name="webCheck">
                            <WebCheck></WebCheck>
                        </q-tab-panel>

                        <q-tab-panel name="bulkCheck" v-if="tokenStore.token">
                            <BulkCheck></BulkCheck>
                        </q-tab-panel>

                        <q-tab-panel name="typoCheck" v-if="tokenStore.token">
                            <TypoCheck></TypoCheck>
                        </q-tab-panel>

                        <q-tab-panel name="login" v-if="!tokenStore.token">
                            <AdminLogin @login-success="loginedUpdate()"></AdminLogin>
                        </q-tab-panel>

                        <q-tab-panel name="setting" v-if="tokenStore.token">
                            <AdminSetting></AdminSetting>
                        </q-tab-panel>
                    </q-tab-panels>
                </div>
            </q-page>
        </q-page-container>
    </q-layout>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount } from "vue";
import { useQuasar } from "quasar";
import { api } from "boot/axios";
import { removeToken } from "src/utils/tokenHandler";
import { close } from "src/utils/websocketHandler";
import { useTokenStore } from "src/stores/tokenStore";
import { useSettingStore } from "src/stores/settingStore";

import WebCheck from "src/components/WebCheck.vue";
import BulkCheck from "src/components/BulkCheck.vue";
import TypoCheck from "src/components/TypoCheck.vue";
import AdminLogin from "src/components/AdminLogin.vue";
import AdminSetting from "src/components/AdminSetting.vue";

defineOptions({
    name: "IndexPage"
});

const $q = useQuasar();
const tokenStore = useTokenStore();
const settingStore = useSettingStore();
const tab = ref("webCheck");

function loginedUpdate() {
    location.reload();
}

function logout() {
    removeToken();
    tab.value = "webCheck";
}

function getWebSettings() {
    api.get("/web/setting")
        .then((response) => {
            if (response.data) {
                settingStore.setSetting(response.data);
            } else {
                $q.notify({
                    position: "top",
                    type: "info",
                    message: "服务器未返回配置参数"
                });
            }
        })
        .catch((error) => {
            console.error("Get settings error: ", error);
            $q.notify({
                position: "top",
                type: "negative",
                message: "获取配置参数失败"
            });
        });
}

onMounted(() => {
    getWebSettings();
});

onBeforeUnmount(() => {
    close();
});
</script>
