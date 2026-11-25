<template>
    <div class="flex flex-center q-py-lg">
        <q-form @submit="onSubmit" ref="loginForm">
            <q-card class="q-px-sm q-pb-sm login-card" flat bordered>
                <q-card-section>
                    <q-input
                        dense
                        outlined
                        class="q-mt-lg"
                        v-model="username"
                        autocomplete="username"
                        type="text"
                        label="用户名"
                        :rules="[$rules.required('请输入用户名')]"
                    ></q-input>
                    <q-input
                        dense
                        outlined
                        class="q-mt-md"
                        v-model="password"
                        autocomplete="current-password"
                        type="password"
                        label="密码"
                        :rules="[$rules.required('请输入密码')]"
                    ></q-input>
                </q-card-section>
                <q-card-section>
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
const submitting = ref(false);

async function onSubmit() {
    if (submitting.value) return;
    submitting.value = true;
    try {
        const { data } = await api.post("/login", { username: username.value, password: password.value });
        setUserName(data.username);
        setToken(data.token);
        send(JSON.stringify({ event: "adminAuth", data: data.token }), false);
        emit("login-success");
    } catch (error) {
        const status = error.response?.status;
        $q.notify({
            position: "top",
            type: "negative",
            message: status === 400 ? "用户名或密码错误" : status === 429 ? "登录尝试过多，请稍后再试" : "登录失败，请检查连接后重试"
        });
    } finally {
        submitting.value = false;
    }
}
</script>

<style scoped>
.login-card {
    width: min(22rem, calc(100vw - 48px));
    border-radius: 8px;
}
</style>
