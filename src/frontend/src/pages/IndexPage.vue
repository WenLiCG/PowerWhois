<template>
    <q-layout class="bg-grey-1">
        <q-page-container>
            <q-page class="workspace">
                <div class="workspace-header-wrap">
                    <header class="workspace-header">
                        <div class="workspace-brand">
                            <img src="/logo.svg" alt="" class="workspace-logo" width="36" height="36" />
                            <div>
                                <h1>PowerWhois</h1>
                                <p>域名即时查询、Typo 衍生与持久化批量任务</p>
                            </div>
                        </div>
                        <div
                            class="connection-state"
                            :class="{
                                'connection-state--open': status === 'OPEN',
                                'connection-state--connecting': status === 'CONNECTING',
                                'connection-state--closed': status !== 'OPEN' && status !== 'CONNECTING'
                            }"
                            role="status"
                        >
                            <q-icon :name="status === 'OPEN' ? 'cloud_done' : status === 'CONNECTING' ? 'sync' : 'cloud_off'" size="18px" />
                            <span>{{ status === "OPEN" ? "服务已连接" : status === "CONNECTING" ? "正在连接服务…" : "连接已断开" }}</span>
                            <q-btn
                                v-if="status !== 'OPEN' && status !== 'CONNECTING'"
                                flat
                                dense
                                no-caps
                                size="sm"
                                color="negative"
                                label="重连"
                                class="q-ml-xs"
                                style="min-height: 24px"
                                @click="open()"
                            />
                        </div>
                    </header>
                </div>
                <div class="workspace-nav-bar full-width">
                    <div class="workspace-nav-inner">
                        <q-tabs
                            v-model="tab"
                            align="left"
                            inline-label
                            no-caps
                            outside-arrows
                            mobile-arrows
                            active-bg-color="light-blue-2"
                            active-color="primary"
                            indicator-color="primary"
                        >
                            <q-tab name="webCheck" icon="search" label="网页查询" />
                            <q-separator vertical inset />
                            <q-tab name="bulkCheck" icon="playlist_add_check" label="批量任务" v-if="tokenStore.token" />
                            <q-separator vertical inset v-if="tokenStore.token" />
                            <q-tab name="typoCheck" icon="spellcheck" label="Typo 查询" v-if="tokenStore.token" />
                            <q-separator vertical inset v-if="!tokenStore.token" />
                            <q-tab name="login" icon="login" label="管理员登录" v-if="!tokenStore.token" />
                        </q-tabs>
                        <q-btn-dropdown
                            auto-close
                            stretch
                            flat
                            no-caps
                            icon="manage_accounts"
                            :label="$q.screen.lt.sm ? undefined : adminLabel"
                            :aria-label="adminLabel"
                            :class="{ 'bg-light-blue-2 text-primary text-weight-medium': tab === 'setting' }"
                            v-if="tokenStore.token"
                        >
                            <q-list style="min-width: 190px; max-width: 90vw">
                                <q-item-label v-if="$q.screen.lt.sm" header style="overflow-wrap: anywhere">{{ adminLabel }}</q-item-label>
                                <q-item clickable @click="tab = 'setting'" :active="tab === 'setting'" active-class="text-primary bg-blue-1">
                                    <q-item-section avatar style="min-width: 32px"><q-icon name="tune" size="18px" /></q-item-section>
                                    <q-item-section>系统设置</q-item-section>
                                </q-item>
                                <q-separator />
                                <q-item clickable :disable="loggingOut" @click="logout">
                                    <q-item-section avatar style="min-width: 32px"><q-icon name="logout" size="18px" color="negative" /></q-item-section>
                                    <q-item-section class="text-negative">退出所有管理会话</q-item-section>
                                </q-item>
                            </q-list>
                        </q-btn-dropdown>
                    </div>
                </div>
                <div class="workspace-content">
                    <q-tab-panels v-model="tab" class="full-width bg-transparent">
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
                            <AdminLogin @login-success="onLogin"></AdminLogin>
                        </q-tab-panel>

                        <q-tab-panel name="setting" v-if="tokenStore.token">
                            <AdminSetting></AdminSetting>
                        </q-tab-panel>
                    </q-tab-panels>
                </div>
                <footer class="workspace-footer">
                    <span>提示：DNS 无委派记录不等于未注册；WHOIS / RDAP 返回“未找到”仍可能为注册局保留词，请以注册商下单结果为准。</span>
                    <a href="/api/openapi.json" target="_blank" rel="noopener">HTTP API 文档（OpenAPI 3.0）</a>
                </footer>
            </q-page>
        </q-page-container>
    </q-layout>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from "vue";
import { useQuasar } from "quasar";
import { api } from "boot/axios";
import { getToken, getUsername, removeToken } from "src/utils/tokenHandler";
import { close, open, status } from "src/utils/websocketHandler";
import { useWebStore } from "src/stores/webStore";
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
const loggingOut = ref(false);

const adminLabel = computed(() => (tokenStore.username ? `管理 (${tokenStore.username})` : "管理"));

function onLogin() {
    getUsername();
    tab.value = "webCheck";
    getWebSettings();
    $q.notify({ position: "top", type: "positive", message: "管理员登录成功" });
}

watch(
    () => tokenStore.token,
    (token, previous) => {
        if (token || !previous) return;
        useWebStore().clearDomains();
        settingStore.clearSetting();
        if (tab.value !== "webCheck") {
            tab.value = "login";
            $q.notify({ type: "warning", message: "管理会话已结束，请重新登录" });
        }
        getWebSettings();
    }
);

async function logout() {
    if (loggingOut.value) return;
    loggingOut.value = true;
    try {
        await api.post("/admin/logout");
    } catch (error) {
        if (error.response?.status !== 401) {
            $q.notify({ type: "negative", message: "退出失败，请重试" });
            return;
        }
    } finally {
        loggingOut.value = false;
    }
    close();
    removeToken();
    open();
    tab.value = "webCheck";
    $q.notify({ position: "top", type: "info", message: "已退出所有管理会话" });
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
        .catch(() => {
            $q.notify({
                position: "top",
                type: "negative",
                message: "获取配置参数失败"
            });
        });
}

onMounted(() => {
    getToken();
    getUsername();
    getWebSettings();
});

onBeforeUnmount(() => {
    close();
});
</script>
