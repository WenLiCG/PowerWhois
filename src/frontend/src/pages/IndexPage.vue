<template>
    <q-layout class="workspace">
        <q-page-container>
            <div class="workspace-frame">
                <a class="skip-link" href="#workspace-main">跳至工作区</a>
                <header class="workspace-masthead">
                    <div class="workspace-header">
                        <div class="workspace-brand"><q-icon name="travel_explore" size="34px" />
                            <span>PowerWhois</span><span class="brand-context">域名工作台</span>
                        </div>
                        <div class="connection-state" role="status" :class="{ 'connection-state--open': status === 'OPEN' }">
                            <span class="connection-dot" aria-hidden="true"></span>
                            {{ status === "OPEN" ? "服务已连接" : status === "CONNECTING" ? "正在连接…" : "连接已断开" }}
                            <q-btn v-if="status !== 'OPEN' && status !== 'CONNECTING'" flat dense no-caps label="重连" @click="open()" />
                        </div>
                    </div>
                </header>
                <nav class="workspace-nav-bar" aria-label="工作台导航">
                    <div class="workspace-nav-inner">
                        <q-tabs :model-value="tab" @update:model-value="selectTab" align="left" inline-label no-caps
                            active-color="primary" indicator-color="primary" narrow-indicator class="workspace-tabs" aria-label="功能">
                            <q-tab name="webCheck" icon="search" label="网页查询" />
                            <q-tab name="bulkCheck" icon="view_list" label="批量任务" />
                            <q-tab name="typoCheck" icon="spellcheck" label="Typo 查询" />
                            <q-tab v-if="!tokenStore.token" name="login" icon="login" label="管理员登录" />
                        </q-tabs>
                        <q-btn-dropdown v-if="tokenStore.token" auto-close flat no-caps icon="account_circle" class="account-menu"
                            :label="$q.screen.lt.sm ? undefined : adminLabel" :aria-label="adminLabel" :color="tab === 'setting' ? 'primary' : undefined">
                            <q-list class="account-options">
                                <q-item-label header>{{ adminLabel }}</q-item-label>
                                <q-item clickable @click="tab = 'setting'" :active="tab === 'setting'">
                                    <q-item-section avatar><q-icon name="tune" /></q-item-section><q-item-section>系统设置</q-item-section>
                                </q-item>
                                <q-separator />
                                <q-item clickable :disable="loggingOut" @click="logout">
                                    <q-item-section avatar><q-icon name="logout" /></q-item-section><q-item-section>退出所有管理会话</q-item-section>
                                </q-item>
                            </q-list>
                        </q-btn-dropdown>
                    </div>
                </nav>
                <main id="workspace-main" class="workspace-content" tabindex="-1">
                    <q-tab-panels v-model="tab" :key="Boolean(tokenStore.token)" keep-alive keep-alive-include="webCheck,typoCheck" class="bg-transparent">
                        <q-tab-panel name="webCheck"><WebCheck /></q-tab-panel>
                        <q-tab-panel v-if="tokenStore.token" name="bulkCheck"><BulkCheck /></q-tab-panel>
                        <q-tab-panel v-if="tokenStore.token" name="typoCheck"><TypoCheck /></q-tab-panel>
                        <q-tab-panel v-if="!tokenStore.token" name="login"><AdminLogin :notice="loginNotice" @login-success="onLogin" /></q-tab-panel>
                        <q-tab-panel v-if="tokenStore.token" name="setting"><AdminSetting @credentials-changed="credentialsChanged" /></q-tab-panel>
                    </q-tab-panels>
                </main>
                <footer class="workspace-footer">
                    <span>DNS 无委派或注册局返回「未找到」时，仍需向注册商核实可注册性。</span>
                    <a href="/api/openapi.json" target="_blank" rel="noopener">HTTP API 文档 <q-icon name="open_in_new" size="14px" /></a>
                </footer>
            </div>
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
const loginNotice = ref("");
let afterLogin = "webCheck";
const loggingOut = ref(false);

const adminLabel = computed(() => (tokenStore.username ? `管理 (${tokenStore.username})` : "管理"));

function selectTab(next) {
    loginNotice.value = "";
    if (!tokenStore.token && (next === "bulkCheck" || next === "typoCheck")) {
        afterLogin = next;
        tab.value = "login";
        loginNotice.value = `登录后将打开${next === "bulkCheck" ? "批量任务" : "Typo 查询"}。`;
        return;
    }
    afterLogin = "webCheck";
    tab.value = next;
}

function onLogin() {
    loginNotice.value = "";
    getUsername();
    tab.value = afterLogin;
    afterLogin = "webCheck";
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
            if (!loginNotice.value) loginNotice.value = "管理会话已结束，请重新登录。";
        }
        getWebSettings();
    }
);

function credentialsChanged() {
    loginNotice.value = "管理员凭据已更新，请使用新凭据登录。";
    afterLogin = "setting";
    close();
    removeToken();
    tab.value = "login";
    open();
}

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
    const token = tokenStore.token;
    api.get("/web/setting")
        .then((response) => {
            if (token !== tokenStore.token) return;
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
            if (token !== tokenStore.token) return;
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
    if (status.value === "CLOSED") open();
    getWebSettings();
});

onBeforeUnmount(() => {
    close();
});
</script>
