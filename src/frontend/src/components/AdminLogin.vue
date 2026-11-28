<template>
    <section class="login-area" aria-labelledby="login-title">
        <q-card flat class="workspace-card login-card">
            <header class="page-heading">
                <q-icon name="lock_outline" class="login-lock" size="24px" aria-hidden="true" />
                <h1 id="login-title">管理员登录</h1>
                <p>登录后可查看批量任务、生成 Typo 域名并管理系统设置。</p>
            </header>
            <p v-if="notice" class="login-notice" role="status">{{ notice }}</p>
            <q-form @submit="onSubmit">
                <q-input
                    outlined v-model="username" for="login-username" name="username"
                    autocomplete="username" autocapitalize="none" autocorrect="off" enterkeyhint="next" :spellcheck="false" required
                    label="用户名" :rules="[$rules.required('请输入用户名')]"
                />
                <q-input
                    outlined v-model="password" for="login-password" name="password"
                    autocomplete="current-password" enterkeyhint="go" required :type="showPassword ? 'text' : 'password'"
                    label="密码" :rules="[(value) => !!value || '请输入密码']"
                >
                    <template #append>
                        <q-btn
                            flat round type="button" size="sm"
                            :icon="showPassword ? 'visibility_off' : 'visibility'"
                            :aria-label="showPassword ? '隐藏密码' : '显示密码'"
                            @click="showPassword = !showPassword"
                        />
                    </template>
                </q-input>
                <p v-if="errorMessage" class="login-error" role="alert">{{ errorMessage }}</p>
                <q-btn unelevated color="primary" no-caps class="full-width login-submit" label="登录" type="submit" :loading="submitting" />
            </q-form>
        </q-card>
    </section>
</template>

<script setup>
import { ref } from "vue";
import { api } from "boot/axios";
import { setUserName, setToken } from "src/utils/tokenHandler";
import { send } from "src/utils/websocketHandler";

defineOptions({ name: "AdminLogin" });
defineProps({ notice: { type: String, default: "" } });
const emit = defineEmits(["login-success"]);
const username = ref("");
const password = ref("");
const showPassword = ref(false);
const submitting = ref(false);
const errorMessage = ref("");

async function onSubmit() {
    if (submitting.value) return;
    submitting.value = true;
    errorMessage.value = "";
    try {
        const { data } = await api.post("/login", { username: username.value.trim(), password: password.value });
        setUserName(data.username);
        setToken(data.token);
        send(JSON.stringify({ event: "adminAuth", data: data.token }), false);
        password.value = "";
        emit("login-success");
    } catch (error) {
        const status = error.response?.status;
        if (status === 400 || status === 401) errorMessage.value = "用户名或密码错误，请重新输入。";
        else if (status === 429) errorMessage.value = "登录尝试过多，请稍后再试。";
        else errorMessage.value = "登录失败，请检查连接后重试。";
    } finally {
        submitting.value = false;
    }
}
</script>

<style scoped>
.login-area {
    display: grid;
    justify-items: center;
    padding: clamp(1rem, 6vw, 5rem) 0;
}
.login-card {
    width: min(100%, 28rem);
    padding: clamp(1.25rem, 4vw, 2.5rem);
}
.login-lock {
    color: var(--q-primary);
    margin-bottom: .875rem;
}
.login-card header p {
    color: #53645e;
    line-height: 1.65;
    margin: .5rem 0 1.5rem;
}
.login-notice, .login-error {
    border-inline-start: 3px solid currentColor;
    padding: .625rem .875rem;
    margin: 0 0 1rem;
}
.login-notice {
    background: #edf5f0;
    color: #086454;
}
.login-error {
    background: #fff1ed;
    color: #963c2c;
}
.login-submit {
    min-height: 44px;
}
</style>
