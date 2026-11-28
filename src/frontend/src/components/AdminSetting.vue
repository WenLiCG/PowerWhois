<template>
    <div v-if="settings">
        <header class="page-heading settings-header">
            <div>
                <h1>系统设置</h1>
                <p>调整管理员账户、查询限额和服务接口。修改后请保存配置。</p>
            </div>
            <div class="settings-actions">
                <q-btn flat no-caps color="primary" icon="refresh" label="放弃修改并重新读取" :disable="submitting || loadingSettings" @click="getSettings" />
                <q-btn unelevated no-caps color="primary" icon="save" label="保存配置" form="powerwhois-settings" type="submit" :loading="submitting" :disable="loadingSettings" />
            </div>
        </header>
        <p v-if="loadFailed" class="settings-warning" role="alert">重新读取失败，已保留当前表单。请检查连接后重试。</p>
        <nav class="settings-nav" aria-label="设置目录">
            <a v-for="section in sections" :key="section.id" :href="'#' + section.id" @click.prevent="focusSection(section.id)">{{ section.name }}</a>
        </nav>

        <q-form id="powerwhois-settings" @validation-error="showValidationError" @submit="onSubmit">
            <!-- 管理员与日志 -->
            <q-card flat class="workspace-card q-mb-md">
                <q-card-section class="settings-section">
                    <h2 id="settings-account" tabindex="-1">账户与日志</h2>
                </q-card-section>
                <q-separator />
                <q-card-section>
                    <div class="row q-col-gutter-md">
                        <div class="col-12 col-md-4">
                            <q-input
                                type="text"
                                outlined
                                dense
                                v-model="settings.authUsername"
                                name="username"
                                for="settings-username"
                                autocomplete="username"
                                autocapitalize="none"
                                autocorrect="off"
                                :spellcheck="false"
                                label="管理员用户名"
                                aria-label="管理员用户名"
                                :disable="settings.secrets?.authUsername?.managed"
                                :hint="settings.secrets?.authUsername?.managed ? '由环境变量 POWERWHOIS_ADMIN_USER 托管' : '用于后台登录的用户名'"
                                :rules="[$rules.required('请输入管理员用户名')]"
                            />
                        </div>
                        <div class="col-12 col-md-4">
                            <q-input
                                type="number"
                                outlined
                                dense
                                suffix="天"
                                v-model.number="settings.authExpireDays"
                                label="登录有效期"
                                aria-label="登录有效期"
                                hint="范围 1–30 天"
                                :rules="[
                                    $rules.required('请设置登录有效期'),
                                    $rules.numeric('登录有效期必须为数字'),
                                    $rules.minValue(1, '登录有效期至少为 1 天'),
                                    $rules.maxValue(30, '登录有效期最多为 30 天')
                                ]"
                            />
                        </div>
                        <div class="col-12 col-md-4">
                            <q-select
                                outlined
                                dense
                                v-model="settings.logLevel"
                                label="运行日志等级"
                                aria-label="日志等级"
                                :options="logLevelOptions"
                            />
                        </div>
                    </div>
                    <div class="password-controls">
                        <div>
                            <div class="text-body2 text-weight-bold">管理员密码</div>
                            <p v-if="settings.secrets?.authPassword?.managed">密码由服务端环境变量托管。</p>
                            <p v-else-if="settings.secretActions.authPassword === 'set'">保存新密码后，所有管理员会话都需要重新登录。</p>
                            <p v-else>保存其他配置时会保留当前密码。</p>
                        </div>
                        <q-btn v-if="!settings.secrets?.authPassword?.managed" outline no-caps color="primary"
                            :label="settings.secretActions.authPassword === 'set' ? '取消更换' : '更换密码'"
                            :aria-expanded="settings.secretActions.authPassword === 'set'"
                            :disable="submitting" @click="togglePassword"
                        />
                    </div>
                    <div v-if="settings.secretActions.authPassword === 'set'" class="new-password">
                        <q-input
                            ref="passwordField"
                            for="settings-new-password"
                            name="new-password"
                            autocomplete="new-password"
                            required
                            :type="showAdminPassword ? 'text' : 'password'"
                            outlined
                            dense
                            v-model="settings.authPassword"
                            label="新管理员密码"
                            aria-label="新管理员密码"
                            :rules="[(value) => !!value?.trim() || '请输入非空白的新密码，或取消更换以保留当前密码']"
                        >
                            <template #append>
                                <q-btn
                                    flat
                                    round
                                    dense
                                    type="button"
                                    :aria-label="showAdminPassword ? '隐藏新密码' : '显示新密码'"
                                    :icon="showAdminPassword ? 'visibility_off' : 'visibility'"
                                    @click="showAdminPassword = !showAdminPassword"
                                />
                            </template>
                        </q-input>
                    </div>

                    <q-separator class="q-my-md" />

                    <div class="row items-center justify-between q-gutter-sm">
                        <div class="text-body2 text-weight-medium">运行日志</div>
                        <div class="row q-gutter-sm">
                            <q-btn
                                outline
                                no-caps
                                color="primary"
                                icon="download"
                                label="下载日志"
                                @click="downloadLog()"
                                :loading="downloadingLog"
                            >
                                <template v-slot:loading>
                                    <q-spinner-hourglass class="on-left" />
                                    打包中…
                                </template>
                            </q-btn>
                            <q-btn outline no-caps color="negative" icon="delete_sweep" label="清空日志" @click="resetLog()" />
                        </div>
                    </div>
                </q-card-section>
            </q-card>

            <!-- 各协议查询限制 -->
            <q-card flat class="workspace-card q-mb-md">
                <q-card-section class="settings-section">
                    <h2 id="settings-protocol" tabindex="-1">协议并发、速率与超时</h2>
                    <p>所有任务共享协议并发。各上游单独限速，并遵循其 Retry-After 冷却时间。</p>
                </q-card-section>
                <q-separator />
                <q-card-section>
                    <div class="row q-col-gutter-md">
                        <div class="col-12 col-sm-6 col-md-4">
                            <q-input
                                type="number"
                                outlined
                                dense
                                suffix="秒"
                                v-model.number="settings.whoisTimeout"
                                label="WHOIS 查询超时"
                                aria-label="WHOIS 查询超时"
                                :rules="[
                                    $rules.required('请设置 Whois 查询超时'),
                                    $rules.numeric('必须为数字'),
                                    $rules.minValue(1, '至少 1 秒'),
                                    $rules.maxValue(120, '最多 120 秒')
                                ]"
                            />
                        </div>
                        <div class="col-12 col-sm-6 col-md-4">
                            <q-input
                                type="number"
                                outlined
                                dense
                                suffix="秒"
                                v-model.number="settings.dnsTimeout"
                                label="DNS 查询超时"
                                aria-label="DNS 查询超时"
                                :rules="[
                                    $rules.required('请设置 DNS 查询超时'),
                                    $rules.numeric('必须为数字'),
                                    $rules.minValue(1, '至少 1 秒'),
                                    $rules.maxValue(120, '最多 120 秒')
                                ]"
                            />
                        </div>
                        <div v-for="field in protocolFields" :key="field.key" class="col-12 col-sm-6 col-md-4">
                            <q-input
                                outlined
                                dense
                                type="number"
                                v-model.number="settings[field.key]"
                                :label="field.label"
                                :min="1"
                                :max="field.max"
                                :rules="[
                                    $rules.required('请填写此参数'),
                                    $rules.numeric('必须为数字'),
                                    $rules.minValue(1, '至少为 1'),
                                    $rules.maxValue(field.max, `最大不超过 ${field.max}`)
                                ]"
                            />
                        </div>
                    </div>

                    <q-separator class="q-my-md" />

                    <div class="row q-col-gutter-md items-center">
                        <div class="col-12 col-sm-4">
                            <q-toggle
                                v-model="settings.retryOnTimeout"
                                label="网页与旧批量开启超时重试"
                                aria-label="超时重试"
                                checked-icon="check"
                                color="primary"
                                unchecked-icon="clear"
                            />
                        </div>
                        <div class="col-12 col-sm-4" v-if="settings.retryOnTimeout">
                            <q-input
                                type="number"
                                outlined
                                dense
                                v-model.number="settings.retryMax"
                                label="最大重试次数 (0–5)"
                                aria-label="重试次数"
                                :rules="[
                                    $rules.required('请设置重试次数'),
                                    $rules.numeric('重试次数必须为数字'),
                                    $rules.minValue(0, '不能为负'),
                                    $rules.maxValue(5, '最多 5 次')
                                ]"
                            />
                        </div>
                        <div class="col-12 col-sm-4" v-if="settings.retryOnTimeout">
                            <q-input
                                type="number"
                                outlined
                                dense
                                suffix="秒"
                                v-model.number="settings.retryInterval"
                                label="重试等待间隔 (0–60s)"
                                aria-label="重试间隔"
                                :rules="[
                                    $rules.required('请设置重试间隔'),
                                    $rules.numeric('必须为数字'),
                                    $rules.minValue(0, '不能为负'),
                                    $rules.maxValue(60, '最多 60 秒')
                                ]"
                            />
                        </div>
                    </div>
                </q-card-section>
            </q-card>

            <!-- 任务与网页并发上限 -->
            <q-card flat class="workspace-card q-mb-md">
                <q-card-section class="settings-section">
                    <h2 id="settings-limits" tabindex="-1">网页与批量任务上限</h2>
                </q-card-section>
                <q-separator />
                <q-card-section>
                    <div class="row q-col-gutter-md">
                        <div class="col-12 col-md-4">
                            <q-input
                                type="number"
                                outlined
                                dense
                                v-model.number="settings.bulkCheckConcurrencyLimit"
                                label="批量任务并发上限 (1–256)"
                                aria-label="批量查询并发上限"
                                :rules="[
                                    $rules.required('请设置并发上限'),
                                    $rules.numeric('必须为数字'),
                                    $rules.minValue(1, '最少为 1'),
                                    $rules.maxValue(256, '最多为 256')
                                ]"
                            />
                        </div>
                        <div class="col-12 col-md-4">
                            <q-input
                                type="number"
                                outlined
                                dense
                                v-model.number="settings.webCheckConcurrencyLimit"
                                label="网页即时查询并发上限 (1–256)"
                                aria-label="网页查询并发上限"
                                :rules="[
                                    $rules.required('请设置并发上限'),
                                    $rules.numeric('必须为数字'),
                                    $rules.minValue(1, '最少为 1'),
                                    $rules.maxValue(256, '最多为 256')
                                ]"
                            />
                        </div>
                        <div class="col-12 col-md-4">
                            <q-input
                                type="number"
                                outlined
                                dense
                                suffix="个"
                                v-model.number="settings.webCheckDomainLimit"
                                label="单次网页查询域名上限"
                                aria-label="单次网页查询域名数量上限"
                                :rules="[
                                    $rules.required('请设置单次查询域名上限'),
                                    $rules.numeric('必须为数字'),
                                    $rules.minValue(1, '至少 1 个域名'),
                                    $rules.maxValue(10000, '最多 10000 个域名')
                                ]"
                            />
                        </div>
                    </div>
                </q-card-section>
            </q-card>

            <!-- SOCKS5 代理与后缀路由 -->
            <q-card flat class="workspace-card q-mb-md">
                <q-card-section class="settings-section row items-center justify-between">
                    <div>
                        <div class="row items-center q-gutter-xs">
                            <h2 id="settings-proxy" tabindex="-1">SOCKS5 代理与后缀分流</h2>
                            <q-badge
                                v-if="settings.socketProxyHost && settings.socketProxyPort > 0"
                                outline
                                color="teal-8"
                            >
                                已配置代理
                            </q-badge>
                            <q-badge v-else outline color="grey-7">未启用代理</q-badge>
                            <q-badge
                                v-if="settings.socketProxyAuth"
                                outline
                                :color="settings.secrets?.socketProxyPassword?.configured ? 'primary' : 'amber-9'"
                            >
                                {{ settings.secrets?.socketProxyPassword?.configured ? "已保存代理密码" : "尚未保存代理密码" }}
                            </q-badge>
                        </div>
                        <p>填写 SOCKS5 主机名或 IP 和端口。域名由代理远程解析。</p>
                    </div>
                    <q-btn
                        outline
                        no-caps
                        size="sm"
                        color="primary"
                        icon="network_check"
                        label="测试 SOCKS5 代理"
                        :disable="!settings.socketProxyHost || testingProxy"
                        :loading="testingProxy"
                        @click="testProxy"
                    />
                </q-card-section>
                <q-separator />
                <q-card-section>
                    <q-banner
                        v-if="proxyTestResult"
                        dense
                        rounded
                        class="q-mb-md"
                        :class="proxyTestResult.ok ? 'bg-green-1 text-green-10' : 'bg-red-1 text-negative'"
                        role="status"
                    >
                        <template #avatar>
                            <q-icon :name="proxyTestResult.ok ? 'check_circle' : 'error_outline'" size="20px" />
                        </template>
                        {{ proxyTestResult.message }}
                        <template #action>
                            <q-btn flat dense no-caps label="关闭" @click="proxyTestResult = null" />
                        </template>
                    </q-banner>
                    <div class="row q-col-gutter-md">
                        <div class="col-12 col-md-5">
                            <q-input
                                type="text"
                                outlined
                                dense
                                clearable
                                v-model="settings.socketProxyHost"
                                label="SOCKS5 代理主机"
                                placeholder="例如 brd.superproxy.io"
                                aria-label="代理地址"
                                hint="留空即禁用代理；勿填写 http:// 或端口号"
                                :rules="[validateProxyHost]"
                            />
                        </div>
                        <div class="col-12 col-sm-6 col-md-4">
                            <q-input
                                type="number"
                                outlined
                                dense
                                v-model.number="settings.socketProxyPort"
                                label="SOCKS5 代理端口"
                                placeholder="例如 22228 或 1080"
                                aria-label="代理端口"
                                :disable="!settings.socketProxyHost"
                                :rules="
                                    !settings.socketProxyHost
                                        ? []
                                        : [
                                              $rules.required('请设置 SOCKS5 代理端口'),
                                              $rules.numeric('端口必须为数字'),
                                              $rules.minValue(1, '端口范围 1–65535'),
                                              $rules.maxValue(65535, '端口范围 1–65535')
                                          ]
                                "
                            />
                        </div>
                        <div class="col-12 col-sm-6 col-md-3 flex items-center">
                            <q-toggle
                                v-model="settings.socketProxyAuth"
                                label="启用代理认证"
                                aria-label="代理认证"
                                :disable="!settings.socketProxyHost"
                                checked-icon="check"
                                color="primary"
                                unchecked-icon="clear"
                                @update:model-value="onProxyAuthToggle"
                            />
                        </div>
                    </div>

                    <div class="row q-col-gutter-md q-mt-xs" v-if="settings.socketProxyHost && settings.socketProxyAuth">
                        <div class="col-12 col-md-4">
                            <q-input
                                type="text"
                                outlined
                                dense
                                v-model="settings.socketProxyUser"
                                label="代理用户名"
                                aria-label="代理用户名"
                                :rules="[$rules.required('请填写代理用户名')]"
                            />
                        </div>
                        <div class="col-12 col-md-4">
                            <q-select
                                outlined
                                dense
                                v-model="settings.secretActions.socketProxyPassword"
                                :options="[
                                    { label: '保持原密码', value: 'keep' },
                                    { label: '设置 / 更换密码', value: 'set' },
                                    { label: '清空密码', value: 'clear' }
                                ]"
                                emit-value
                                map-options
                                label="代理密码操作"
                            />
                        </div>
                        <div class="col-12 col-md-4">
                            <q-input
                                :type="showProxyPassword ? 'text' : 'password'"
                                outlined
                                dense
                                v-model="settings.socketProxyPassword"
                                label="代理密码"
                                aria-label="代理密码"
                                :disable="settings.secretActions.socketProxyPassword !== 'set'"
                                :hint="settings.secrets?.socketProxyPassword?.configured ? '原密码不回显；选择“设置 / 更换密码”后输入新密码' : '首次启用认证请填写代理密码'"
                                :rules="[(v) => settings.secretActions.socketProxyPassword !== 'set' || !!v || '请输入代理密码']"
                            >
                                <template #append>
                                    <q-btn
                                        v-if="settings.secretActions.socketProxyPassword === 'set'"
                                        flat
                                        round
                                        dense
                                        type="button"
                                        :aria-label="showProxyPassword ? '隐藏代理密码' : '显示代理密码'"
                                        :icon="showProxyPassword ? 'visibility_off' : 'visibility'"
                                        @click="showProxyPassword = !showProxyPassword"
                                    />
                                </template>
                            </q-input>
                        </div>
                    </div>

                    <q-separator class="q-my-md" />

                    <div class="row q-col-gutter-md">
                        <div class="col-12 col-md-4">
                            <q-input
                                type="textarea"
                                outlined
                                dense
                                clearable
                                autogrow
                                :input-style="{ minHeight: '84px' }"
                                label="全局强制代理后缀（每行一个）"
                                placeholder="例如：&#10;cn&#10;hk"
                                v-model="settings.globalProxyTldsInput"
                            />
                        </div>
                        <div class="col-12 col-md-4">
                            <q-input
                                type="textarea"
                                outlined
                                dense
                                clearable
                                autogrow
                                :input-style="{ minHeight: '84px' }"
                                label="Mixed 模式走代理后缀（每行一个）"
                                placeholder="例如：&#10;co&#10;au"
                                v-model="settings.mixedProxyTldsInput"
                            />
                        </div>
                        <div class="col-12 col-md-4">
                            <q-input
                                type="textarea"
                                outlined
                                dense
                                clearable
                                autogrow
                                :input-style="{ minHeight: '84px' }"
                                label="Mixed 模式走 DNS 后缀（每行一个）"
                                placeholder="例如：&#10;de&#10;ch&#10;ac.uk"
                                v-model="settings.mixedDnsTldsInput"
                            />
                        </div>
                    </div>
                </q-card-section>
            </q-card>

            <!-- Typo 查询参数 -->
            <q-card flat class="workspace-card q-mb-md">
                <q-card-section class="settings-section">
                    <h2 id="settings-typo" tabindex="-1">Typo 默认后缀与替换词</h2>
                </q-card-section>
                <q-separator />
                <q-card-section>
                    <div class="text-body2 text-weight-medium q-mb-xs">默认后缀列表（勾选表示在 Typo 页面默认选中）</div>
                    <div class="row items-center q-gutter-sm q-mb-sm">
                        <div v-for="tld in ccTldOptions" :key="tld" class="tld-pill row items-center no-wrap">
                            <q-checkbox dense v-model="selectedCcTlds" :val="tld" :label="tld" color="primary" />
                            <q-btn round flat dense size="xs" color="negative" icon="close" :aria-label="`移除 ${tld}`" @click="removeCcTld(tld)" />
                        </div>
                        <q-input
                            dense
                            outlined
                            v-model="inputCcTld"
                            placeholder="新增后缀（回车添加）"
                            style="width: 210px"
                            @keydown.enter.prevent="addCcTld"
                        >
                            <template v-slot:after>
                                <q-btn dense unelevated icon="add" aria-label="添加后缀" color="primary" :disable="!inputCcTld" @click="addCcTld" />
                            </template>
                        </q-input>
                    </div>

                    <q-separator class="q-my-md" />

                    <q-input
                        type="textarea"
                        outlined
                        dense
                        clearable
                        autogrow
                        label="自定义 Typo 字符替换规则（每行一条，格式 原字符:替换字符）"
                        placeholder="s2:ss22&#10;sa:sal"
                        v-model="settings.typoCustomizedReplacesInput"
                        lazy-rules
                        :rules="[validateCustomizedReplaces]"
                    />
                </q-card-section>
            </q-card>

            <!-- 注册接口设定 -->
            <q-card flat class="workspace-card q-mb-md">
                <q-card-section class="settings-section row items-center">
                    <div>
                        <h2 id="settings-register" tabindex="-1">自定义注册商接口</h2>
                        <p>管理员确认注册后才会调用接口。保存过的地址会隐藏。</p>
                    </div>
                    <q-space />
                    <q-btn unelevated no-caps color="primary" size="sm" icon="add" label="添加注册接口" @click="openAddRegisterApiDialog()" />
                </q-card-section>
                <q-separator />
                <q-card-section>
                    <q-markup-table
                        flat
                        bordered
                        wrap-cells
                        separator="cell"
                        class="full-width"
                        v-if="settings.registerApis && settings.registerApis.length > 0"
                    >
                        <thead>
                            <tr>
                                <th class="text-left">名称</th>
                                <th class="text-left">接口地址状态</th>
                                <th class="text-left">成功 / 失败标识</th>
                                <th class="text-center">并发上限</th>
                                <th class="text-center">操作</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="apiItem in settings.registerApis" :key="apiItem.apiName">
                                <td class="text-left text-weight-medium">{{ apiItem.apiName }}</td>
                                <td class="text-left">
                                    <q-badge v-if="apiItem.apiUrl" color="primary" outline>已填写新地址（保存后生效）</q-badge>
                                    <q-badge v-else-if="apiItem.urlConfigured" color="teal-8" outline>已配置（地址安全隐藏）</q-badge>
                                    <q-badge v-else color="negative" outline>未配置地址</q-badge>
                                </td>
                                <td class="text-left text-caption">
                                    成功: {{ (apiItem.successText || []).join(", ") }}<br />
                                    失败: {{ (apiItem.failText || []).join(", ") }}
                                </td>
                                <td class="text-center">{{ apiItem.concurrencyLimit }}</td>
                                <td class="text-center q-gutter-xs">
                                    <q-btn flat dense no-caps color="primary" icon="edit" label="编辑" @click="openUpdateRegisterApiDialog(apiItem)" />
                                    <q-btn flat dense no-caps color="negative" icon="delete" label="删除" @click="deleteRegisterApi(apiItem)" />
                                </td>
                            </tr>
                        </tbody>
                    </q-markup-table>
                    <div class="text-center text-grey-7 q-py-md" v-else>暂无自定义注册接口</div>
                </q-card-section>
            </q-card>

            <!-- 自定义 Whois 接口设定 -->
            <q-card flat class="workspace-card q-mb-md">
                <q-card-section class="settings-section row items-center">
                    <div>
                        <h2 id="settings-whois" tabindex="-1">自定义 WHOIS 查询接口</h2>
                        <p>网页、批量任务、CLI 和 MCP 均可按名称调用。</p>
                    </div>
                    <q-space />
                    <q-btn unelevated no-caps color="primary" size="sm" icon="add" label="添加查询接口" @click="openAddWhoisApiDialog()" />
                </q-card-section>
                <q-separator />
                <q-card-section>
                    <q-markup-table
                        flat
                        bordered
                        wrap-cells
                        separator="cell"
                        class="full-width"
                        v-if="settings.whoisApis && settings.whoisApis.length > 0"
                    >
                        <thead>
                            <tr>
                                <th class="text-left">名称</th>
                                <th class="text-left">接口地址状态</th>
                                <th class="text-left">未注册 / 已注册标识</th>
                                <th class="text-center">并发上限</th>
                                <th class="text-center">操作</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="apiItem in settings.whoisApis" :key="apiItem.apiName">
                                <td class="text-left text-weight-medium">{{ apiItem.apiName }}</td>
                                <td class="text-left">
                                    <q-badge v-if="apiItem.apiUrl" color="primary" outline>已填写新地址（保存后生效）</q-badge>
                                    <q-badge v-else-if="apiItem.urlConfigured" color="teal-8" outline>已配置（地址安全隐藏）</q-badge>
                                    <q-badge v-else color="negative" outline>未配置地址</q-badge>
                                </td>
                                <td class="text-left text-caption">
                                    未注册: {{ (apiItem.freeText || []).join(", ") }}<br />
                                    已注册: {{ (apiItem.takenText || []).join(", ") }}
                                </td>
                                <td class="text-center">{{ apiItem.concurrencyLimit }}</td>
                                <td class="text-center q-gutter-xs">
                                    <q-btn flat dense no-caps color="primary" icon="edit" label="编辑" @click="openUpdateWhoisApiDialog(apiItem)" />
                                    <q-btn flat dense no-caps color="negative" icon="delete" label="删除" @click="deleteWhoisApi(apiItem)" />
                                </td>
                            </tr>
                        </tbody>
                    </q-markup-table>
                    <div class="text-center text-grey-7 q-py-md" v-else>暂无自定义查询接口</div>
                </q-card-section>
            </q-card>

            <div class="row q-col-gutter-sm q-py-md">
                <div class="col">
                    <q-btn unelevated no-caps color="primary" class="full-width" icon="save" label="保存全部系统配置" type="submit" :loading="submitting" :disable="loadingSettings">
                        <template v-slot:loading>
                            <q-spinner-dots />
                        </template>
                    </q-btn>
                </div>
            </div>
        </q-form>
    </div>
    <q-banner v-else-if="loadFailed" class="bg-red-1 text-negative rounded-borders" role="alert">
        无法读取管理设置，请检查连接后重试。
        <template #action><q-btn flat no-caps label="重新加载" @click="getSettings" /></template>
    </q-banner>
    <div class="q-pa-lg flex flex-center" v-else role="status" aria-label="正在加载管理设置">
        <q-spinner color="primary" size="3em" :thickness="10" />
    </div>

    <!-- 注册接口弹窗 -->
    <q-dialog persistent v-model="displayRegisterApiEditDialog">
        <q-card class="api-editor">
            <q-form @submit.prevent="updateRegisterApiItem.apiName ? updateRegisterApi() : addRegisterApi()">
                <q-card-section class="row items-center q-py-sm">
                    <div class="text-subtitle1 text-weight-bold" v-if="updateRegisterApiItem.apiName">
                        编辑注册接口：{{ updateRegisterApiItem.apiName }}
                    </div>
                    <div class="text-subtitle1 text-weight-bold" v-else>添加注册接口</div>
                    <q-space />
                    <q-btn icon="close" aria-label="关闭编辑" flat round dense v-close-popup />
                </q-card-section>

                <q-separator />

                <q-card-section class="q-gutter-md">
                    <q-input
                        outlined
                        dense
                        v-model="inputRegisterApiData.apiName"
                        label="接口名称"
                        :rules="[$rules.required('请设置接口名称')]"
                    />
                    <q-input
                        outlined
                        dense
                        v-model="inputRegisterApiData.apiUrl"
                        type="textarea"
                        autogrow
                        label="API URL（使用 {domain} 代表域名）"
                        placeholder="https://registrar.example/api/register?domain={domain}"
                        hint="已有同名接口留空表示保持原地址；新建或修改接口名称时必须填写完整 HTTP(S) 地址（不含账号密码与 # 片段）"
                    />
                    <q-input
                        type="number"
                        outlined
                        dense
                        v-model.number="inputRegisterApiData.concurrencyLimit"
                        label="并发数限制 (1–256)"
                        :rules="[
                            $rules.required('请设置并发限制'),
                            $rules.numeric('必须为数字'),
                            $rules.minValue(1, '最少为 1'),
                            $rules.maxValue(256, '最多为 256')
                        ]"
                    />
                    <div>
                        <div class="row items-center justify-between q-mb-xs">
                            <span class="text-body2 text-weight-medium">成功响应匹配词（至少一项）</span>
                            <q-btn flat dense no-caps size="sm" color="primary" icon="add" label="添加匹配词" @click="addSuccessText" />
                        </div>
                        <div v-for="(_, index) in inputRegisterApiData.successText" :key="index" class="row items-center no-wrap q-gutter-xs q-mb-xs">
                            <q-input
                                outlined
                                dense
                                class="col"
                                v-model="inputRegisterApiData.successText[index]"
                                :placeholder="`成功标识 #${index + 1}`"
                                :rules="[$rules.required('不能为空')]"
                            />
                            <q-btn round flat dense color="negative" icon="close" :aria-label="`移除成功标识 ${index + 1}`" @click="removeSuccessText(index)" />
                        </div>
                    </div>
                    <div>
                        <div class="row items-center justify-between q-mb-xs">
                            <span class="text-body2 text-weight-medium">失败响应匹配词（至少一项，不能与成功标识相同）</span>
                            <q-btn flat dense no-caps size="sm" color="primary" icon="add" label="添加匹配词" @click="addFailText" />
                        </div>
                        <div v-for="(_, index) in inputRegisterApiData.failText" :key="index" class="row items-center no-wrap q-gutter-xs q-mb-xs">
                            <q-input
                                outlined
                                dense
                                class="col"
                                v-model="inputRegisterApiData.failText[index]"
                                :placeholder="`失败标识 #${index + 1}`"
                                :rules="[$rules.required('不能为空')]"
                            />
                            <q-btn round flat dense color="negative" icon="close" :aria-label="`移除失败标识 ${index + 1}`" @click="removeFailText(index)" />
                        </div>
                    </div>
                </q-card-section>

                <q-separator />

                <q-card-actions align="right">
                    <q-btn flat no-caps color="grey-8" label="取消" v-close-popup />
                    <q-btn unelevated no-caps color="primary" type="submit" :label="updateRegisterApiItem.apiName ? '确认修改' : '确认添加'" />
                </q-card-actions>
            </q-form>
        </q-card>
    </q-dialog>

    <!-- Whois 接口弹窗 -->
    <q-dialog persistent v-model="displayWhoisApiEditDialog">
        <q-card class="api-editor">
            <q-form @submit.prevent="updateWhoisApiItem.apiName ? updateWhoisApi() : addWhoisApi()">
                <q-card-section class="row items-center q-py-sm">
                    <div class="text-subtitle1 text-weight-bold" v-if="updateWhoisApiItem.apiName">
                        编辑查询接口：{{ updateWhoisApiItem.apiName }}
                    </div>
                    <div class="text-subtitle1 text-weight-bold" v-else>添加自定义查询接口</div>
                    <q-space />
                    <q-btn icon="close" aria-label="关闭编辑" flat round dense v-close-popup />
                </q-card-section>

                <q-separator />

                <q-card-section class="q-gutter-md">
                    <q-input
                        outlined
                        dense
                        v-model="inputWhoisApiData.apiName"
                        label="接口名称"
                        :rules="[$rules.required('请设置接口名称')]"
                    />
                    <q-input
                        outlined
                        dense
                        v-model="inputWhoisApiData.apiUrl"
                        type="textarea"
                        autogrow
                        label="API URL（使用 {domain} 代表域名）"
                        placeholder="https://whois.example/api?domain={domain}"
                        hint="已有同名接口留空表示保持原地址；新建或修改接口名称时必须填写完整 HTTP(S) 地址"
                    />
                    <q-input
                        type="number"
                        outlined
                        dense
                        v-model.number="inputWhoisApiData.concurrencyLimit"
                        label="并发数限制 (1–256)"
                        :rules="[
                            $rules.required('请设置并发限制'),
                            $rules.numeric('必须为数字'),
                            $rules.minValue(1, '最少为 1'),
                            $rules.maxValue(256, '最多为 256')
                        ]"
                    />
                    <div>
                        <div class="row items-center justify-between q-mb-xs">
                            <span class="text-body2 text-weight-medium">未找到（Free）响应匹配词（至少一项）</span>
                            <q-btn flat dense no-caps size="sm" color="primary" icon="add" label="添加匹配词" @click="addFreeText" />
                        </div>
                        <div v-for="(_, index) in inputWhoisApiData.freeText" :key="index" class="row items-center no-wrap q-gutter-xs q-mb-xs">
                            <q-input
                                outlined
                                dense
                                class="col"
                                v-model="inputWhoisApiData.freeText[index]"
                                :placeholder="`未找到标识 #${index + 1}`"
                                :rules="[$rules.required('不能为空')]"
                            />
                            <q-btn round flat dense color="negative" icon="close" :aria-label="`移除未找到标识 ${index + 1}`" @click="removeFreeText(index)" />
                        </div>
                    </div>
                    <div>
                        <div class="row items-center justify-between q-mb-xs">
                            <span class="text-body2 text-weight-medium">已注册（Taken）响应匹配词（至少一项，不能与未找到标识相同）</span>
                            <q-btn flat dense no-caps size="sm" color="primary" icon="add" label="添加匹配词" @click="addTakenText" />
                        </div>
                        <div v-for="(_, index) in inputWhoisApiData.takenText" :key="index" class="row items-center no-wrap q-gutter-xs q-mb-xs">
                            <q-input
                                outlined
                                dense
                                class="col"
                                v-model="inputWhoisApiData.takenText[index]"
                                :placeholder="`已注册标识 #${index + 1}`"
                                :rules="[$rules.required('不能为空')]"
                            />
                            <q-btn round flat dense color="negative" icon="close" :aria-label="`移除已注册标识 ${index + 1}`" @click="removeTakenText(index)" />
                        </div>
                    </div>
                </q-card-section>

                <q-separator />

                <q-card-actions align="right">
                    <q-btn flat no-caps color="grey-8" label="取消" v-close-popup />
                    <q-btn unelevated no-caps color="primary" type="submit" :label="updateWhoisApiItem.apiName ? '确认修改' : '确认添加'" />
                </q-card-actions>
            </q-form>
        </q-card>
    </q-dialog>
</template>

<script setup>
defineOptions({
    name: "AdminSetting"
});

import { nextTick, ref, onMounted } from "vue";
import { useQuasar, is, date } from "quasar";
import { api } from "boot/axios";
import { useSettingStore } from "src/stores/settingStore";

const $q = useQuasar();
const emit = defineEmits(["credentials-changed"]);
const sections = [
    { id: "settings-account", name: "账户与日志" },
    { id: "settings-protocol", name: "协议参数" },
    { id: "settings-limits", name: "任务上限" },
    { id: "settings-proxy", name: "代理分流" },
    { id: "settings-typo", name: "Typo" },
    { id: "settings-register", name: "注册接口" },
    { id: "settings-whois", name: "查询接口" }
];

function focusSection(id) {
    document.getElementById(id)?.focus();
}
const protocolFields = [
    { key: "rdapTimeout", label: "RDAP 超时 / 秒", max: 120 },
    ...[
        ["dns", "DNS"],
        ["whois", "WHOIS"],
        ["rdap", "RDAP"]
    ].flatMap(([key, label]) => [
        { key: `${key}Concurrency`, label: `${label} 最大并发`, max: 256 },
        { key: `${key}Rate`, label: `${label} 每上游每秒请求数`, max: 256 }
    ])
];

const settingStore = useSettingStore();

const settings = ref(null);
let savedProxyHost = "";
let savedUsername = "";
const loadFailed = ref(false);
const loadingSettings = ref(false);
const logLevelOptions = ["Debug", "Info", "Warn", "Error", "Off"];

const submitting = ref(false);
const downloadingLog = ref(false);
const testingProxy = ref(false);
const proxyTestResult = ref(null);
const showAdminPassword = ref(false);
const passwordField = ref(null);
const showProxyPassword = ref(false);

const ccTldOptions = ref([]);
const selectedCcTlds = ref([]);
const inputCcTld = ref("");

const displayRegisterApiEditDialog = ref(false);
const displayWhoisApiEditDialog = ref(false);

const updateRegisterApiItem = ref({
    apiName: "",
    apiUrl: "",
    urlConfigured: false,
    successText: [],
    failText: [],
    concurrencyLimit: 1
});
const updateWhoisApiItem = ref({
    apiName: "",
    apiUrl: "",
    urlConfigured: false,
    freeText: [],
    takenText: [],
    concurrencyLimit: 1
});

const inputRegisterApiData = ref({
    apiName: "",
    apiUrl: "",
    successText: [""],
    failText: [""],
    concurrencyLimit: 1
});
const inputWhoisApiData = ref({
    apiName: "",
    apiUrl: "",
    freeText: [""],
    takenText: [""],
    concurrencyLimit: 1
});

const reservedWhoisApiNames = ref([
    "whois",
    "rdap",
    "dns",
    "mixed",
    "proxy",
    "whoisQuery",
    "whoisQueryWithProxy",
    "dnsQuery",
    "mixedQuery"
]);

async function togglePassword() {
    const change = settings.value.secretActions.authPassword !== "set";
    settings.value.authPassword = "";
    settings.value.secretActions.authPassword = change ? "set" : "keep";
    showAdminPassword.value = false;
    if (change) {
        await nextTick();
        passwordField.value?.focus();
    }
}

function getSettings() {
    if (loadingSettings.value || submitting.value) return;
    loadingSettings.value = true;
    loadFailed.value = false;
    proxyTestResult.value = null;
    api.get("/admin/setting")
        .then((response) => {
            if (response.data) {
                settings.value = response.data;
                savedProxyHost = response.data.socketProxyHost || "";
                savedUsername = response.data.authUsername;
                showAdminPassword.value = false;
                settings.value.secretActions = { authPassword: "keep", jwtSecretKey: "keep", socketProxyPassword: "keep" };
                settings.value.globalProxyTldsInput = settings.value.globalProxyTlds ? settings.value.globalProxyTlds.join("\n") : "";
                settings.value.mixedProxyTldsInput = settings.value.mixedProxyTlds ? settings.value.mixedProxyTlds.join("\n") : "";
                settings.value.mixedDnsTldsInput = settings.value.mixedDnsTlds ? settings.value.mixedDnsTlds.join("\n") : "";

                selectedCcTlds.value = [];
                ccTldOptions.value = [];
                if (settings.value.typoDefaultCcTlds) {
                    settings.value.typoDefaultCcTlds.forEach((tld) => {
                        if (tld.isSelected) {
                            selectedCcTlds.value.push(tld.tld);
                        }
                        ccTldOptions.value.push(tld.tld);
                    });
                }

                settings.value.typoCustomizedReplacesInput = settings.value.typoCustomizedReplaces
                    ? settings.value.typoCustomizedReplaces.join("\n")
                    : "";
            } else {
                loadFailed.value = true;
            }
        })
        .catch(() => {
            loadFailed.value = true;
        })
        .finally(() => {
            loadingSettings.value = false;
        });
}

function showValidationError() {
    $q.notify({
        position: "top",
        type: "negative",
        message: "存在未通过校验的配置项，请检查表单红字提示"
    });
}

function resetLog() {
    $q.dialog({
        title: "清空运行日志",
        message: "是否确认清空当前服务日志？此操作不可恢复。",
        cancel: "取消",
        ok: "确认清空",
        persistent: true
    }).onOk(() => {
        api.delete("/admin/log")
            .then(() => {
                $q.notify({
                    position: "top",
                    type: "positive",
                    message: "当前日志已清空"
                });
            })
            .catch(() => {
                $q.notify({
                    position: "top",
                    type: "negative",
                    message: "清空日志失败"
                });
            });
    });
}

function onProxyAuthToggle(enabled) {
    if (enabled && settings.value && !settings.value.secrets?.socketProxyPassword?.configured) {
        settings.value.secretActions.socketProxyPassword = "set";
    }
}

function confirmProxyPasswordReuse(payload, testing) {
    const host = payload.socketProxyHost;
    if (!host || host.toLowerCase() === savedProxyHost.toLowerCase() || payload.secretActions.socketProxyPassword !== "keep" ||
        !settings.value.secrets?.socketProxyPassword?.configured || (testing && !payload.socketProxyAuth)) return true;

    const destination = host.includes(":") && !host.startsWith("[") ? `[${host}]:${payload.socketProxyPort}` : `${host}:${payload.socketProxyPort}`;
    return new Promise((resolve) => {
        $q.dialog({
            title: "向新的代理主机沿用已存密码？",
            message: testing
                ? `测试连接会向 ${destination} 发送已保存的代理密码。`
                : `保存后，启用代理认证的连接会向 ${destination} 发送已保存的密码。`,
            cancel: "返回修改",
            ok: testing ? "沿用密码并测试" : "沿用密码并保存",
            focus: "cancel"
        }).onOk(() => resolve(true)).onDismiss(() => resolve(false));
    });
}

function validateProxyHost(value) {
    const host = (value || "").trim();
    if (!host) return true;
    const lower = host.toLowerCase();
    if (lower.startsWith("http://") || lower.startsWith("https://")) {
        return "仅支持 SOCKS5 代理，请填写主机名或 IP（不含 http://）";
    }
    if (/[\s/@?#]/.test(host)) {
        return "请填写纯主机名或 IP（不含路径、空格或认证信息）";
    }
    return true;
}

async function testProxy() {
    if (!settings.value || testingProxy.value) return;
    const host = (settings.value.socketProxyHost || "").trim();
    const hostCheck = validateProxyHost(host);
    if (!host || hostCheck !== true) {
        proxyTestResult.value = { ok: false, message: hostCheck === true ? "请先填写 SOCKS5 代理主机" : hostCheck };
        return;
    }
    const port = Number(settings.value.socketProxyPort);
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
        proxyTestResult.value = { ok: false, message: "请填写有效的 SOCKS5 代理端口（1–65535）" };
        return;
    }
    if (settings.value.socketProxyAuth && !(settings.value.socketProxyUser || "").trim()) {
        proxyTestResult.value = { ok: false, message: "启用代理认证时请先填写代理用户名" };
        return;
    }
    testingProxy.value = true;
    proxyTestResult.value = null;
    try {
        const payload = {
            socketProxyHost: host,
            socketProxyPort: port,
            socketProxyAuth: Boolean(settings.value.socketProxyAuth),
            socketProxyUser: (settings.value.socketProxyUser || "").trim(),
            socketProxyPassword: settings.value.socketProxyPassword || "",
            secretActions: { socketProxyPassword: settings.value.secretActions.socketProxyPassword }
        };
        if (!(await confirmProxyPasswordReuse(payload, true))) return;
        const { data } = await api.post("/admin/proxy/test", payload, { timeout: 18000 });
        proxyTestResult.value = {
            ok: true,
            message: `SOCKS5 代理连通测试成功（耗时 ${data.latencyMs} ms，已验证目标 ${data.target} TLS 握手）。别忘了保存系统配置使其生效。`
        };
    } catch (error) {
        proxyTestResult.value = {
            ok: false,
            message: error.response?.data?.message || "代理连通性测试失败，请检查代理主机、端口与凭据"
        };
    } finally {
        testingProxy.value = false;
    }
}

function downloadLog() {
    const defaultFilename = "log_" + date.formatDate(Date.now(), "YYYY-MM-DD_HH-mm-ss") + ".zip";
    downloadingLog.value = true;

    api.get("/admin/log", {
        responseType: "blob",
        timeout: 600000
    })
        .then((response) => {
            const disposition = response.headers["content-disposition"] || "";
            const match = /filename=([^;]+)/i.exec(disposition);
            const serverFilename = match ? match[1].trim().replace(/^["']|["']$/g, "") : defaultFilename;

            const url = window.URL.createObjectURL(new Blob([response.data]));
            const link = document.createElement("a");
            link.href = url;
            link.setAttribute("download", serverFilename);
            document.body.appendChild(link);
            link.click();

            setTimeout(() => {
                document.body.removeChild(link);
                window.URL.revokeObjectURL(url);
            }, 100);

            downloadingLog.value = false;
        })
        .catch(() => {
            $q.notify({
                position: "top",
                type: "negative",
                message: "下载日志文件失败，请稍后重试"
            });
            downloadingLog.value = false;
        });
}

function addCcTld() {
    if (inputCcTld.value) {
        const newTld = inputCcTld.value.replace(/\s+/g, "").replace(/^\.+/, "").replace(/\.+$/, "").toLowerCase();
        if (newTld.length > 0) {
            if (ccTldOptions.value.includes(newTld)) {
                $q.notify({
                    position: "top",
                    type: "warning",
                    message: "该后缀已存在"
                });
            } else {
                ccTldOptions.value.push(newTld);
                selectedCcTlds.value.push(newTld);
                inputCcTld.value = "";
            }
        } else {
            $q.notify({
                position: "top",
                type: "warning",
                message: "输入的后缀无效"
            });
        }
    }
}

function removeCcTld(tld) {
    ccTldOptions.value = ccTldOptions.value.filter((item) => item != tld);
    selectedCcTlds.value = selectedCcTlds.value.filter((item) => item != tld);
}

function validateCustomizedReplaces(value) {
    if (value) {
        const lines = value
            .split(/\r?\n/)
            .map((line) => line.trim())
            .filter(Boolean);
        for (const line of lines) {
            const parts = line.split(":");
            if (parts.length !== 2 || !parts[0].trim() || !parts[1].trim()) {
                return "格式有误，每行一条：原字符:替换字符";
            }
        }
    }
    return true;
}

function validateCustomApiPayload(name, urlValue, keepExistingUrl, positiveList, negativeList) {
    const cleanUrl = (urlValue || "").trim();
    if (!cleanUrl && !keepExistingUrl) {
        $q.notify({ position: "top", type: "negative", message: "新建接口或修改接口名称时必须填写 API URL" });
        return null;
    }
    if (cleanUrl) {
        try {
            const parsed = new URL(cleanUrl.replaceAll("{domain}", "example.com"));
            if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || !parsed.hostname || parsed.username || parsed.password || parsed.hash) {
                throw new Error("bad url");
            }
        } catch {
            $q.notify({
                position: "top",
                type: "negative",
                message: "API URL 必须为合法的 http:// 或 https:// 地址，且不含内嵌账号密码或 # 片段"
            });
            return null;
        }
    }
    const pos = positiveList.map((s) => (s || "").trim()).filter(Boolean);
    const neg = negativeList.map((s) => (s || "").trim()).filter(Boolean);
    if (pos.length === 0 || neg.length === 0) {
        $q.notify({ position: "top", type: "negative", message: "请至少填写各一项非空匹配词" });
        return null;
    }
    for (const p of pos) {
        if (neg.includes(p)) {
            $q.notify({ position: "top", type: "negative", message: `正向与反向匹配词不能包含相同内容：“${p}”` });
            return null;
        }
    }
    return { apiName: name.trim(), apiUrl: cleanUrl, pos, neg };
}

function addSuccessText() {
    inputRegisterApiData.value.successText.push("");
}
function removeSuccessText(index) {
    inputRegisterApiData.value.successText.splice(index, 1);
}
function addFailText() {
    inputRegisterApiData.value.failText.push("");
}
function removeFailText(index) {
    inputRegisterApiData.value.failText.splice(index, 1);
}

function openAddRegisterApiDialog() {
    displayRegisterApiEditDialog.value = true;
    updateRegisterApiItem.value = {
        apiName: "",
        apiUrl: "",
        urlConfigured: false,
        successText: [],
        failText: [],
        concurrencyLimit: 1
    };
    inputRegisterApiData.value = {
        apiName: "",
        apiUrl: "",
        successText: [""],
        failText: [""],
        concurrencyLimit: 1
    };
}

function addRegisterApi() {
    if (!settings.value.registerApis) {
        settings.value.registerApis = [];
    }
    const name = (inputRegisterApiData.value.apiName || "").trim();
    if (settings.value.registerApis.find((item) => item.apiName == name)) {
        $q.notify({ position: "top", type: "negative", message: "注册接口名称已存在，请使用其他名称" });
        return;
    }
    const checked = validateCustomApiPayload(
        name,
        inputRegisterApiData.value.apiUrl,
        false,
        inputRegisterApiData.value.successText,
        inputRegisterApiData.value.failText
    );
    if (!checked) return;
    settings.value.registerApis.push({
        apiName: checked.apiName,
        apiUrl: checked.apiUrl,
        urlConfigured: true,
        successText: checked.pos,
        failText: checked.neg,
        concurrencyLimit: inputRegisterApiData.value.concurrencyLimit
    });
    displayRegisterApiEditDialog.value = false;
}

function openUpdateRegisterApiDialog(apiItem) {
    displayRegisterApiEditDialog.value = true;
    updateRegisterApiItem.value = apiItem;
    inputRegisterApiData.value = {
        apiName: apiItem.apiName,
        apiUrl: apiItem.apiUrl || "",
        successText: apiItem.successText?.length ? [...apiItem.successText] : [""],
        failText: apiItem.failText?.length ? [...apiItem.failText] : [""],
        concurrencyLimit: apiItem.concurrencyLimit
    };
}

function updateRegisterApi() {
    const name = (inputRegisterApiData.value.apiName || "").trim();
    if (settings.value.registerApis.find((item) => item.apiName == name && item.apiName != updateRegisterApiItem.value.apiName)) {
        $q.notify({ position: "top", type: "negative", message: "注册接口名称已存在，请使用其他名称" });
        return;
    }
    const canKeepExistingUrl = name === updateRegisterApiItem.value.apiName && !!updateRegisterApiItem.value.urlConfigured;
    const checked = validateCustomApiPayload(
        name,
        inputRegisterApiData.value.apiUrl,
        canKeepExistingUrl,
        inputRegisterApiData.value.successText,
        inputRegisterApiData.value.failText
    );
    if (!checked) return;

    const targetApi = settings.value.registerApis.find((item) => item.apiName == updateRegisterApiItem.value.apiName);
    if (targetApi) {
        targetApi.apiName = checked.apiName;
        targetApi.apiUrl = checked.apiUrl;
        targetApi.urlConfigured = Boolean(checked.apiUrl || canKeepExistingUrl);
        targetApi.successText = checked.pos;
        targetApi.failText = checked.neg;
        targetApi.concurrencyLimit = inputRegisterApiData.value.concurrencyLimit;
    }
    displayRegisterApiEditDialog.value = false;
}

function deleteRegisterApi(apiItem) {
    settings.value.registerApis = settings.value.registerApis.filter((item) => item.apiName != apiItem.apiName);
}

function addFreeText() {
    inputWhoisApiData.value.freeText.push("");
}
function removeFreeText(index) {
    inputWhoisApiData.value.freeText.splice(index, 1);
}
function addTakenText() {
    inputWhoisApiData.value.takenText.push("");
}
function removeTakenText(index) {
    inputWhoisApiData.value.takenText.splice(index, 1);
}

function openAddWhoisApiDialog() {
    displayWhoisApiEditDialog.value = true;
    updateWhoisApiItem.value = {
        apiName: "",
        apiUrl: "",
        urlConfigured: false,
        freeText: [],
        takenText: [],
        concurrencyLimit: 1
    };
    inputWhoisApiData.value = {
        apiName: "",
        apiUrl: "",
        freeText: [""],
        takenText: [""],
        concurrencyLimit: 1
    };
}

function addWhoisApi() {
    if (!settings.value.whoisApis) {
        settings.value.whoisApis = [];
    }
    const name = (inputWhoisApiData.value.apiName || "").trim();
    if (reservedWhoisApiNames.value.includes(name)) {
        $q.notify({ position: "top", type: "negative", message: `${name} 是系统保留的查询模式名称，请使用其他名称` });
        return;
    }
    if (settings.value.whoisApis.find((item) => item.apiName == name)) {
        $q.notify({ position: "top", type: "negative", message: "Whois 接口名称已存在，请使用其他名称" });
        return;
    }
    const checked = validateCustomApiPayload(
        name,
        inputWhoisApiData.value.apiUrl,
        false,
        inputWhoisApiData.value.freeText,
        inputWhoisApiData.value.takenText
    );
    if (!checked) return;

    settings.value.whoisApis.push({
        apiName: checked.apiName,
        apiUrl: checked.apiUrl,
        urlConfigured: true,
        freeText: checked.pos,
        takenText: checked.neg,
        concurrencyLimit: inputWhoisApiData.value.concurrencyLimit
    });
    displayWhoisApiEditDialog.value = false;
}

function openUpdateWhoisApiDialog(apiItem) {
    displayWhoisApiEditDialog.value = true;
    updateWhoisApiItem.value = apiItem;
    inputWhoisApiData.value = {
        apiName: apiItem.apiName,
        apiUrl: apiItem.apiUrl || "",
        freeText: apiItem.freeText?.length ? [...apiItem.freeText] : [""],
        takenText: apiItem.takenText?.length ? [...apiItem.takenText] : [""],
        concurrencyLimit: apiItem.concurrencyLimit
    };
}

function updateWhoisApi() {
    const name = (inputWhoisApiData.value.apiName || "").trim();
    if (reservedWhoisApiNames.value.includes(name)) {
        $q.notify({ position: "top", type: "negative", message: `${name} 是系统保留的查询模式名称，请使用其他名称` });
        return;
    }
    if (settings.value.whoisApis.find((item) => item.apiName == name && item.apiName != updateWhoisApiItem.value.apiName)) {
        $q.notify({ position: "top", type: "negative", message: "Whois 接口名称已存在，请使用其他名称" });
        return;
    }
    const canKeepExistingUrl = name === updateWhoisApiItem.value.apiName && !!updateWhoisApiItem.value.urlConfigured;
    const checked = validateCustomApiPayload(
        name,
        inputWhoisApiData.value.apiUrl,
        canKeepExistingUrl,
        inputWhoisApiData.value.freeText,
        inputWhoisApiData.value.takenText
    );
    if (!checked) return;

    const targetApi = settings.value.whoisApis.find((item) => item.apiName == updateWhoisApiItem.value.apiName);
    if (targetApi) {
        targetApi.apiName = checked.apiName;
        targetApi.apiUrl = checked.apiUrl;
        targetApi.urlConfigured = Boolean(checked.apiUrl || canKeepExistingUrl);
        targetApi.freeText = checked.pos;
        targetApi.takenText = checked.neg;
        targetApi.concurrencyLimit = inputWhoisApiData.value.concurrencyLimit;
    }
    displayWhoisApiEditDialog.value = false;
}

function deleteWhoisApi(apiItem) {
    settings.value.whoisApis = settings.value.whoisApis.filter((item) => item.apiName != apiItem.apiName);
}

function splitNonEmptyLines(text) {
    if (!text) return [];
    return text
        .split(/\r?\n/)
        .map((s) => s.trim())
        .filter(Boolean);
}

async function onSubmit() {
    if (submitting.value || loadingSettings.value) return;
    submitting.value = true;
    try {
        if (!settings.value.retryOnTimeout) {
            if (!is.number(settings.value.retryInterval)) settings.value.retryInterval = 0;
            if (!is.number(settings.value.retryMax)) settings.value.retryMax = 0;
        }

        settings.value.globalProxyTlds = splitNonEmptyLines(settings.value.globalProxyTldsInput);
        settings.value.mixedProxyTlds = splitNonEmptyLines(settings.value.mixedProxyTldsInput);
        settings.value.mixedDnsTlds = splitNonEmptyLines(settings.value.mixedDnsTldsInput);
        settings.value.typoDefaultCcTlds = ccTldOptions.value.map((tld) => ({
            tld,
            isSelected: selectedCcTlds.value.includes(tld)
        }));
        settings.value.typoCustomizedReplaces = splitNonEmptyLines(settings.value.typoCustomizedReplacesInput);

        const cleanHost = (settings.value.socketProxyHost || "").trim();
        const payload = {
            ...settings.value,
            secretActions: { ...settings.value.secretActions },
            authPassword: settings.value.secretActions.authPassword === "set" ? settings.value.authPassword : "",
            socketProxyHost: cleanHost,
            socketProxyPort: cleanHost ? settings.value.socketProxyPort || 0 : 0,
            socketProxyAuth: cleanHost ? Boolean(settings.value.socketProxyAuth) : false
        };
        if (!(await confirmProxyPasswordReuse(payload, false))) return;
        const { data } = await api.put("/admin/setting", payload);
        settings.value.socketProxyHost = data.socketProxyHost || "";
        savedProxyHost = settings.value.socketProxyHost;
        settings.value.socketProxyPort = data.socketProxyPort || 0;
        settings.value.socketProxyAuth = Boolean(data.socketProxyAuth);
        settings.value.authPassword = "";
        settings.value.socketProxyPassword = "";
        settings.value.jwtSecretKey = "";
        settings.value.secretActions = { authPassword: "keep", jwtSecretKey: "keep", socketProxyPassword: "keep" };
        settings.value.secrets = data.secrets;
        for (const key of ["whoisApis", "registerApis"]) settings.value[key] = data[key];
        if (payload.secretActions.authPassword === "set" || data.authUsername !== savedUsername) {
            emit("credentials-changed");
            return;
        }
        settingStore.updateSetting(data);
        $q.notify({ position: "top", type: "positive", message: "系统配置保存成功" });
    } catch (error) {
        $q.notify({
            position: "top",
            type: "negative",
            message:
                error.response?.status === 400 && typeof error.response.data === "string"
                    ? "保存失败：" + error.response.data
                    : "未收到保存结果，请检查连接后重试"
        });
    } finally {
        submitting.value = false;
    }
}

onMounted(() => {
    getSettings();
});
</script>

<style scoped>
.settings-header, .settings-actions, .password-controls {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: .75rem;
}
.settings-header p, .password-controls p, .settings-section p {
    color: #53645e;
    line-height: 1.6;
    margin: .25rem 0 0;
}
.settings-nav {
    display: flex;
    flex-wrap: wrap;
    gap: .25rem .875rem;
    margin: 1rem 0;
    border-block: 1px solid #d7dfd7;
    padding: .375rem 0;
}
.settings-nav a {
    display: inline-flex;
    align-items: center;
    min-height: 36px;
    color: var(--q-primary);
    text-underline-offset: .22em;
}
.settings-section {
    gap: .75rem;
}
.settings-section h2 {
    color: #182b27;
    font-size: 1.075rem;
    line-height: 1.4;
    font-weight: 650;
    letter-spacing: 0;
    margin: 0;
    scroll-margin-top: 6rem;
}
.password-controls {
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid #dee5df;
}
.new-password {
    max-width: 33rem;
    margin-top: 1rem;
}
.settings-warning {
    color: #963c2c;
}
.api-editor {
    width: 38rem;
    max-width: 94vw;
}
.tld-pill {
    padding: 3px 6px 3px 8px;
    border: 1px solid #d7dfd7;
    border-radius: 6px;
    background: #f5f7f3;
}
</style>
