<template>
    <div class="flex flex-center q-py-lg">
        <q-form @submit="onSubmit" ref="loginForm">
            <q-card class="workspace-card q-pa-md login-card" flat>
                <q-card-section class="q-pb-sm">
                    <div class="text-h6 text-weight-bold text-primary">管理员登录</div>
                    <div class="text-caption text-grey-7 q-mt-xs">登录后可使用批量任务、Typo 查询、代理路由与系统设置</div>
                </q-card-section>
                <q-card-section class="q-pt-sm">
                    <q-input
                        dense
                        outlined
                        autofocus
                        v-model="username"
                        autocomplete="username"
                        type="text"
                        label="用户名"
                        :rules="[$rules.required('请输入用户名')]"
                    >
                        <template #prepend><q-icon name="person" size="18px" /></template>
                    </q-input>
                    <q-input
                        dense
                        outlined
                        class="q-mt-sm"
                        v-model="password"
                        autocomplete="current-password"
                        :type="showPassword ? 'text' : 'password'"
                        label="密码"
                        :rules="[$rules.required('请输入密码')]"
                    >
                        <template #prepend><q-icon name="lock" size="18px" /></template>
                        <template #append>
                            <q-btn
                                flat
                                round
                                dense
                                tabindex="-1"
                                :icon="showPassword ? 'visibility_off' : 'visibility'"
                                :aria-label="showPassword ? '隐藏密码' : '显示密码'"
                                @click="showPassword = !showPassword"
                            />
                        </template>
                    </q-input>
                </q-card-section>
                <q-card-section class="q-pt-none">
                    <q-btn
                        unelevated
                        color="primary"
                        size="md"
                        label="登录"
                        no-caps
                        class="full-width"
                        type="submit"
                        :loading="submitting"
                    >
                        <template v-slot:loading>
                            <q-spinner-dots />
                        </template>
                    </q-btn>
                </q-card-section>
            </q-card>
        </q-form>
    </div>
</template>

<script setup>
defineOptions({
    name: "AdminLogin"
});

import { ref } from "vue";
import { useQuasar } from "quasar";
import { api } from "boot/axios";
import { setUserName, setToken } from "src/utils/tokenHandler";
import { send } from "src/utils/websocketHandler";

const emit = defineEmits(["login-success"]);

const $q = useQuasar();

const loginForm = ref(null);
const username = ref("");
const password = ref("");
const showPassword = ref(false);
const submitting = ref(false);

async function onSubmit() {
    if (submitting.value) return;
    submitting.value = true;
    try {
        const { data } = await api.post("/login", { username: username.value.trim(), password: password.value });
        setUserName(data.username);
        setToken(data.token);
        send(JSON.stringify({ event: "adminAuth", data: data.token }), false);
        emit("login-success");
    } catch (error) {
        const status = error.response?.status;
        $q.notify({
            position: "top",
            type: "negative",
            message:
                status === 400 || status === 401
                    ? "用户名或密码错误"
                    : status === 429
                      ? "登录尝试过多，请稍后再试"
                      : "登录失败，请检查连接后重试"
        });
    } finally {
        submitting.value = false;
    }
}
</script>

<style scoped>
.login-card {
    width: min(24rem, calc(100vw - 32px));
}
</style>
