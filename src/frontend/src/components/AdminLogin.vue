<template>
    <div class="flex flex-center q-py-lg">
        <q-form @submit="onSubmit" ref="loginForm">
            <q-card class="q-px-sm q-pb-lg login-card" bordered>
                <q-card-section>
                    <q-input
                        dense
                        outlined
                        class="q-mt-lg"
                        v-model="username"
                        type="text"
                        label="用户名"
                        :rules="[$rules.required('请输入用户名')]"
                    ></q-input>
                    <q-input
                        dense
                        outlined
                        class="q-mt-md"
                        v-model="password"
                        type="password"
                        label="密码"
                        :rules="[$rules.required('请输入密码')]"
                    ></q-input>
                </q-card-section>
                <q-card-section>
                    <q-btn
                        style="border-radius: 8px"
                        color="green"
                        rounded
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

const emit = defineEmits(["login-success"]);

const $q = useQuasar();

const loginForm = ref(null);
const username = ref("");
const password = ref("");
const submitting = ref(false);

async function onSubmit() {
    if (await loginForm.value.validate()) {
        submitting.value = true;
        api.post("/login", {
            username: username.value,
            password: password.value
        })
            .then((response) => {
                setUserName(response.data.username);
                setToken(response.data.token);
                submitting.value = false;
                emit("login-success");
            })
            .catch((error) => {
                submitting.value = false;
                // 401 from this release, 400 from older backends.
                if (error.response && [400, 401].includes(error.response.status)) {
                    $q.notify({
                        position: "top",
                        type: "negative",
                        message: "登录失败, 请检查用户名或密码."
                    });
                } else {
                    $q.notify({
                        position: "top",
                        type: "negative",
                        message: error.response?.status === 429 ? "登录尝试过多，请稍后再试" : "登录失败，请稍后重试"
                    });
                }
            });
    }
}
</script>

<style>
.login-card {
    width: 22rem;
    border-radius: 8px;
    box-shadow: 0 20px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1);
}
</style>
