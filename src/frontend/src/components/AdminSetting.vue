<template>
    <div v-if="settings">
        <p class="text-caption text-grey-8">密码至少 12 位，登录有效期为 1–30 天。通过环境变量设置的凭据需在服务端修改并重启。</p>
        <q-form @validation-error="showValidationError" @submit="onSubmit" ref="settingForm">
            <!-- 管理员配置 -->
            <q-card class="no-shadow q-pb-lg" bordered>
                <q-card-section class="row items-center q-px-lg">
                    <div class="text-subtitle2 text-center">管理员配置</div>
                </q-card-section>

                <q-separator></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">管理员用户名</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="text"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model="settings.authUsername" aria-label="管理员用户名"
                                :rules="[$rules.required('请输入管理员用户名')]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">管理员密码</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-select outlined dense v-model="settings.secretActions.authPassword" :options="[{label:'保持原密码',value:'keep'},{label:'更换密码',value:'set'}]" emit-value map-options label="密码操作" :disable="settings.secrets?.authPassword?.managed" />
                            <q-input
                                type="password"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model="settings.authPassword" aria-label="管理员密码"
                                :disable="settings.secrets?.authPassword?.managed || settings.secretActions.authPassword !== 'set'"
                                minlength="12"
                                hint="密码不会回显；更换时输入新密码。环境凭据需在服务端更换。"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">登录有效期</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                suffix="天"
                                v-model.number="settings.authExpireDays" aria-label="登录有效期"
                                :rules="[
                                    $rules.required('请设置登录有效期'),
                                    $rules.numeric('登录有效期必须为数字'),
                                    $rules.minValue(1, '登录有效期至少为1天'),
                                    $rules.maxValue(30, '登录有效期最多为30天')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-separator inset></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">日志等级</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-select outlined dense round item-aligned v-model="settings.logLevel" aria-label="日志等级" :options="logLevelOptions" />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right"></q-item-section>
                    </q-item>
                    <q-item class="col-8">
                        <div class="q-gutter-md q-pl-md">
                            <q-btn color="negative" icon="fa-solid fa-trash-can" label="清空日志" @click="resetLog()"></q-btn>
                            <q-btn
                                color="primary"
                                icon-right="fa-solid fa-cloud-arrow-down"
                                label="下载日志"
                                @click="downloadLog()"
                                :loading="downloadingLog"
                            >
                                <template v-slot:loading>
                                    <q-spinner-hourglass class="on-left" />
                                    正在下载...
                                </template>
                            </q-btn>
                        </div>
                    </q-item>
                </q-card-section>
            </q-card>

            <q-card flat bordered class="q-my-md">
                <q-card-section><h2 class="text-subtitle1 q-ma-none">各协议查询限制</h2><p class="text-caption text-grey-8">并发在整个服务内共享；请求频率按上游服务器限制，收到 Retry-After 后等待。</p>
                    <div class="row q-col-gutter-md">
                        <div v-for="field in protocolFields" :key="field.key" class="col-12 col-sm-6 col-md-4">
                            <q-input outlined dense type="number" v-model.number="settings[field.key]" :label="field.label" :min="1" :max="field.max" :rules="[$rules.minValue(1, '至少为 1'), $rules.maxValue(field.max, '超过上限')]" />
                        </div>
                    </div>
                </q-card-section>
            </q-card>
            <!-- 公共查询参数 -->
            <q-card class="no-shadow q-mt-md q-pb-lg" bordered>
                <q-card-section class="row items-center q-px-lg">
                    <div class="text-subtitle2 text-center">公共查询参数</div>
                </q-card-section>

                <q-separator></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">Whois查询超时</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                suffix="秒"
                                v-model.number="settings.whoisTimeout" aria-label="WHOIS 查询超时"
                                :rules="[
                                    $rules.required('请设置Whois查询超时'),
                                    $rules.numeric('Whois查询超时必须为数字'),
                                    $rules.minValue(1, 'Whois查询超时至少为1秒'),
                                    $rules.maxValue(120, 'Whois查询超时最多为120秒')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">DNS查询超时</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                suffix="秒"
                                v-model.number="settings.dnsTimeout" aria-label="DNS 查询超时"
                                :rules="[
                                    $rules.required('请设置DNS查询超时'),
                                    $rules.numeric('DNS查询超时必须为数字'),
                                    $rules.minValue(1, 'DNS查询超时至少为1秒'),
                                    $rules.maxValue(120, 'DNS查询超时最多为120秒')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">超时重试</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section class="q-pl-sm">
                            <q-toggle v-model="settings.retryOnTimeout" aria-label="超时重试" checked-icon="check" color="blue" unchecked-icon="clear"></q-toggle>
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm" v-if="settings.retryOnTimeout">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">重试次数</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model.number="settings.retryMax" aria-label="重试次数"
                                :rules="[
                                    $rules.required('请设置重试次数'),
                                    $rules.numeric('重试次数必须为数字'),
                                    $rules.minValue(0, '重试次数不能为负'),
                                    $rules.maxValue(5, '重试次数最多为5次')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm" v-if="settings.retryOnTimeout">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">重试间隔</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                suffix="秒"
                                v-model.number="settings.retryInterval" aria-label="重试间隔"
                                :rules="[
                                    $rules.required('请设置失败重试间隔'),
                                    $rules.numeric('失败重试间隔必须为数字'),
                                    $rules.minValue(0, '失败重试间隔不能为负'),
                                    $rules.maxValue(60, '失败重试间隔最多为60秒')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-separator inset></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">全局强制使用代理查询的域名后缀</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="textarea"
                                outlined
                                dense
                                round
                                clearable
                                item-aligned
                                placeholder="请输入通过代理查询的域名后缀, 每行一个域名后缀"
                                v-model="settings.globalProxyTldsInput"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-separator inset></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">混合模式下使用代理查询的域名后缀</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="textarea"
                                outlined
                                dense
                                round
                                clearable
                                item-aligned
                                placeholder="请输入通过代理查询的域名后缀, 每行一个域名后缀"
                                v-model="settings.mixedProxyTldsInput"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">混合模式下使用DNS查询的域名后缀</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="textarea"
                                outlined
                                dense
                                round
                                clearable
                                item-aligned
                                placeholder="请输入通过DNS查询的域名后缀, 每行一个域名后缀"
                                v-model="settings.mixedDnsTldsInput"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-separator inset></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">Socks代理地址</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="text"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model="settings.socketProxyHost" aria-label="代理地址"
                                hint="留空可禁用代理"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">Socks代理端口</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model.number="settings.socketProxyPort" aria-label="代理端口"
                                :disable="!settings.socketProxyHost"
                                :rules="!settings.socketProxyHost ? [] : [
                                    $rules.required('请设置Socks代理端口'),
                                    $rules.numeric('Socks代理端口必须为数字'),
                                    $rules.minValue(1, 'Socks代理端口错误'),
                                    $rules.maxValue(65535, 'Socks代理端口错误')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">Socks代理认证</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section class="q-pl-sm">
                            <q-toggle v-model="settings.socketProxyAuth" aria-label="代理认证" :disable="!settings.socketProxyHost" checked-icon="check" color="blue" unchecked-icon="clear"></q-toggle>
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm" v-if="settings.socketProxyHost && settings.socketProxyAuth">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">Socks代理用户</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="text"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model="settings.socketProxyUser" aria-label="代理用户名"
                                :rules="[$rules.required('请设置Socks代理用户')]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm" v-if="settings.socketProxyHost && settings.socketProxyAuth">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">Socks代理密码</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-select outlined dense v-model="settings.secretActions.socketProxyPassword" :options="[{label:'保持',value:'keep'},{label:'更换',value:'set'},{label:'清空',value:'clear'}]" emit-value map-options label="代理密码操作" />
                            <q-input
                                type="password"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model="settings.socketProxyPassword" aria-label="代理密码"
                                :disable="settings.secretActions.socketProxyPassword !== 'set'"
                                hint="只有更换时需要填写；清空请选择“清空”"
                                :rules="[v => settings.secretActions.socketProxyPassword !== 'set' || !!v || '请输入新代理密码']"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>
            </q-card>

            <!-- 批量查询参数 -->
            <q-card class="no-shadow q-mt-md q-pb-lg" bordered>
                <q-card-section class="row items-center q-px-lg">
                    <div class="text-subtitle2 text-center">批量查询参数</div>
                </q-card-section>

                <q-separator></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">并发任务数</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model.number="settings.bulkCheckConcurrencyLimit" aria-label="批量查询并发上限"
                                :rules="[
                                    $rules.required('请设置并发任务数'),
                                    $rules.numeric('并发任务数必须为数字'),
                                    $rules.minValue(1, '并发任务数最少为1'),
                                    $rules.maxValue(256, '并发任务数最多为256')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>
            </q-card>

            <!-- 网页查询参数 -->
            <q-card class="no-shadow q-mt-md q-pb-lg" bordered>
                <q-card-section class="row items-center q-px-lg">
                    <div class="text-subtitle2 text-center">网页查询参数</div>
                </q-card-section>

                <q-separator></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">并发任务数</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                v-model.number="settings.webCheckConcurrencyLimit" aria-label="网页查询并发上限"
                                :rules="[
                                    $rules.required('请设置并发任务数'),
                                    $rules.numeric('并发任务数必须为数字'),
                                    $rules.minValue(1, '并发任务数最少为1'),
                                    $rules.maxValue(256, '并发任务数最多为256')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">单次查询域名数量限制</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="number"
                                outlined
                                dense
                                round
                                item-aligned
                                suffix="个"
                                v-model.number="settings.webCheckDomainLimit" aria-label="单次网页查询域名数量上限"
                                :rules="[
                                    $rules.required('请设置单次查询域名数量限制'),
                                    $rules.numeric('单次查询域名数量限制必须为数字'),
                                    $rules.minValue(1, '至少查询 1 个域名'),
                                    $rules.maxValue(10000, '每次最多查询 10000 个域名')
                                ]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>
            </q-card>

            <!-- Typo查询参数 -->
            <q-card class="no-shadow q-mt-md q-pb-lg" bordered>
                <q-card-section class="row items-center q-px-lg">
                    <div class="text-subtitle2 text-center">Typo查询参数</div>
                </q-card-section>

                <q-separator></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">默认ccTLDs</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm q-pl-lg">
                        <q-item-section>
                            <div class="flex justify-left q-gutter-sm" v-for="tld in ccTldOptions" :key="tld">
                                <q-checkbox v-model="selectedCcTlds" :val="tld" :label="tld" color="cyan" />
                                <q-btn round flat color="negative" icon="close" @click="removeCcTld(tld)" />
                            </div>
                            <div class="flex justify-left q-pt-xs q-pb-md q-px-sm">
                                <q-input dense outlined v-model="inputCcTld" placeholder="添加更多TLD">
                                    <template v-slot:after>
                                        <q-btn icon="add" color="primary" @click="addCcTld" />
                                    </template>
                                </q-input>
                            </div>
                        </q-item-section>
                    </q-item>
                </q-card-section>

                <q-separator inset></q-separator>

                <q-card-section class="row q-pa-sm">
                    <q-item class="col-4 q-pa-sm">
                        <q-item-section class="text-right">自定义Typo替换</q-item-section>
                    </q-item>
                    <q-item class="col-8 q-pa-sm">
                        <q-item-section>
                            <q-input
                                type="textarea"
                                outlined
                                dense
                                round
                                clearable
                                item-aligned
                                placeholder="请输入自定义Typo替换, 每行一个替换, 格式: 原字符:替换字符"
                                v-model="settings.typoCustomizedReplacesInput"
                                lazy-rules
                                :rules="[validateCustomizedReplaces]"
                            />
                        </q-item-section>
                    </q-item>
                </q-card-section>
            </q-card>

            <!-- 注册接口设定 -->
            <q-card class="no-shadow q-mt-md q-pb-lg" bordered>
                <q-card-section class="row items-center q-px-lg">
                    <div class="text-subtitle2 text-center">注册接口设定</div>
                    <q-space />
                    <div class="text-caption text-center">
                        <q-btn color="primary" size="sm" icon="add" label="添加" @click="openAddRegisterApiDialog()" />
                    </div>
                </q-card-section>

                <q-separator></q-separator>

                <q-card-section class="row q-pa-sm flex flex-center">
                    <q-markup-table
                        flat
                        bordered
                        wrap-cells
                        separator="cell"
                        class="full-width"
                        v-if="settings.registerApis && settings.registerApis.length > 0"
                    >
                        <thead style="position: sticky; top: 0; background: #e3f2fd; z-index: 1">
                            <tr>
                                <th class="text-center" style="min-width: 80px">名称</th>
                                <th class="text-center">URL</th>
                                <th class="text-center" style="min-width: 90px">并发限制</th>
                                <th class="text-center" style="min-width: 90px">操作</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="apiItem in settings.registerApis" :key="apiItem.apiName">
                                <td class="text-center">{{ apiItem.apiName }}</td>
                                <td class="text-left">{{ apiItem.apiUrl || "已配置（地址不回显）" }}</td>
                                <td class="text-center">{{ apiItem.concurrencyLimit }}</td>
                                <td class="text-center q-gutter-sm">
                                    <q-btn color="primary" size="sm" icon="edit" label="修改" @click="openUpdateRegisterApiDialog(apiItem)" />
                                    <q-btn color="negative" size="sm" icon="delete" label="删除" @click="deleteRegisterApi(apiItem)" />
                                </td>
                            </tr>
                        </tbody>
                    </q-markup-table>
                    <div class="text-center" v-else>
                        <q-icon name="info" size="md" color="primary" />
                        <div class="text-caption">暂无注册接口</div>
                    </div>
                </q-card-section>
            </q-card>

            <!-- 自定义Whois接口设定 -->
            <q-card class="no-shadow q-mt-md q-pb-lg" bordered>
                <q-card-section class="row items-center q-px-lg">
                    <div class="text-subtitle2 text-center">自定义Whois接口</div>
                    <q-space />
                    <div class="text-caption text-center">
                        <q-btn color="primary" size="sm" icon="add" label="添加" @click="openAddWhoisApiDialog()" />
                    </div>
                </q-card-section>

                <q-separator></q-separator>

                <q-card-section class="row q-pa-sm flex flex-center">
                    <q-markup-table
                        flat
                        bordered
                        wrap-cells
                        separator="cell"
                        class="full-width"
                        v-if="settings.whoisApis && settings.whoisApis.length > 0"
                    >
                        <thead style="position: sticky; top: 0; background: #e3f2fd; z-index: 1">
                            <tr>
                                <th class="text-center" style="min-width: 80px">名称</th>
                                <th class="text-center">URL</th>
                                <th class="text-center" style="min-width: 90px">并发限制</th>
                                <th class="text-center" style="min-width: 90px">操作</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr v-for="apiItem in settings.whoisApis" :key="apiItem.apiName">
                                <td class="text-center">{{ apiItem.apiName }}</td>
                                <td class="text-left">{{ apiItem.apiUrl || "已配置（地址不回显）" }}</td>
                                <td class="text-center">{{ apiItem.concurrencyLimit }}</td>
                                <td class="text-center q-gutter-sm">
                                    <q-btn color="primary" size="sm" icon="edit" label="修改" @click="openUpdateWhoisApiDialog(apiItem)" />
                                    <q-btn color="negative" size="sm" icon="delete" label="删除" @click="deleteWhoisApi(apiItem)" />
                                </td>
                            </tr>
                        </tbody>
                    </q-markup-table>
                    <div class="text-center" v-else>
                        <q-icon name="info" size="md" color="primary" />
                        <div class="text-caption">暂无自定义Whois接口</div>
                    </div>
                </q-card-section>
            </q-card>

            <div class="q-py-lg">
                <q-btn color="primary" class="full-width" icon="save" label="保存" type="submit" :loading="submitting">
                    <template v-slot:loading>
                        <q-spinner-dots />
                    </template>
                </q-btn>
            </div>
        </q-form>
    </div>
    <q-banner v-else-if="loadFailed" class="bg-red-1 text-negative" role="alert">
        无法读取管理设置，请检查连接后重试。
        <template #action><q-btn flat label="重新加载" @click="getSettings" /></template>
    </q-banner>
    <div class="q-pa-lg flex flex-center" v-else role="status" aria-label="正在加载管理设置">
        <q-spinner color="primary" size="3em" :thickness="10" />
    </div>

    <q-dialog persistent v-model="displayRegisterApiEditDialog">
        <q-card class="api-editor">
            <q-card-section class="row items-center">
                <div class="text-subtitle2" v-if="updateRegisterApiItem.apiName">修改注册接口: {{ updateRegisterApiItem.apiName }}</div>
                <div class="text-subtitle2" v-else>添加注册接口</div>
                <q-space />
                <q-btn icon="close" aria-label="关闭编辑" flat round dense v-close-popup />
            </q-card-section>

            <q-separator></q-separator>

            <q-card-section>
                <div class="q-pt-md">
                    <q-markup-table flat separator="cell">
                        <tbody>
                            <tr>
                                <td class="text-right">API名称</td>
                                <td class="text-left">
                                    <q-input
                                        outlined
                                        round
                                        dense
                                        hide-bottom-space
                                        v-model="inputRegisterApiData.apiName"
                                        lazy-rules
                                        :rules="[$rules.required('请设置API名称')]"
                                    />
                                </td>
                            </tr>
                            <tr>
                                <td class="text-right">API URL</td>
                                <td class="text-left">
                                    <q-input
                                        outlined
                                        round
                                        dense
                                        hide-bottom-space
                                        v-model="inputRegisterApiData.apiUrl"
                                        type="textarea"
                                        placeholder="填写新地址，{domain} 为域名占位符；已有接口留空保持原地址，重命名需重填"
                                        lazy-rules
                                        :rules="[]"
                                    />
                                </td>
                            </tr>
                            <tr>
                                <td class="text-right">成功标识字符</td>
                                <td class="text-left">
                                    <div
                                        v-for="(textItem, index) in inputRegisterApiData.successText"
                                        :key="index"
                                        class="q-gutter-md row items-center q-py-xs"
                                    >
                                        <q-chip color="info" text-color="white" size="sm">{{ index + 1 }}</q-chip>
                                        <q-input
                                            outlined
                                            round
                                            dense
                                            hide-bottom-space
                                            v-model="inputRegisterApiData.successText[index]"
                                            lazy-rules
                                            :rules="[$rules.required('请设置成功标识字符')]"
                                        />
                                        <q-btn round flat color="negative" icon="close" @click="removeSuccessText(index)" />
                                    </div>
                                    <div class="q-pt-sm">
                                        <q-btn size="sm" color="primary" label="添加" @click="addSuccessText" class="items-center q-px-lg" />
                                    </div>
                                </td>
                            </tr>
                            <tr>
                                <td class="text-right">失败标识字符</td>
                                <td class="text-left">
                                    <div
                                        v-for="(textItem, index) in inputRegisterApiData.failText"
                                        :key="index"
                                        class="q-gutter-md row items-center q-py-xs"
                                    >
                                        <q-chip color="info" text-color="white" size="sm">{{ index + 1 }}</q-chip>
                                        <q-input
                                            outlined
                                            round
                                            dense
                                            hide-bottom-space
                                            v-model="inputRegisterApiData.failText[index]"
                                            lazy-rules
                                            :rules="[$rules.required('请设置失败标识字符')]"
                                        />
                                        <q-btn round flat color="negative" icon="close" @click="removeFailText(index)" />
                                    </div>
                                    <div class="q-pt-sm">
                                        <q-btn size="sm" color="primary" label="添加" @click="addFailText" class="items-center q-px-lg" />
                                    </div>
                                </td>
                            </tr>
                            <tr>
                                <td class="text-right">并发数限制</td>
                                <td class="text-left">
                                    <q-input
                                        type="number"
                                        outlined
                                        round
                                        dense
                                        hide-bottom-space
                                        v-model.number="inputRegisterApiData.concurrencyLimit"
                                        lazy-rules
                                        :rules="[
                                            $rules.required('请设置并发限制'),
                                            $rules.numeric('并发限制必须为数字'),
                                            $rules.minValue(1, '并发限制最少为1')
                                        ]"
                                    />
                                </td>
                            </tr>
                        </tbody>
                    </q-markup-table>
                </div>
            </q-card-section>

            <q-separator></q-separator>

            <q-card-actions align="right">
                <q-btn color="info" label="取消" v-close-popup />
                <q-btn color="green" label="修改" @click="updateRegisterApi()" v-if="updateRegisterApiItem.apiName" />
                <q-btn color="green" label="添加" @click="addRegisterApi()" v-else />
            </q-card-actions>
        </q-card>
    </q-dialog>

    <q-dialog persistent v-model="displayWhoisApiEditDialog">
        <q-card class="api-editor">
            <q-card-section class="row items-center">
                <div class="text-subtitle2" v-if="updateWhoisApiItem.apiName">修改Whois接口: {{ updateWhoisApiItem.apiName }}</div>
                <div class="text-subtitle2" v-else>添加Whois接口</div>
                <q-space />
                <q-btn icon="close" aria-label="关闭编辑" flat round dense v-close-popup />
            </q-card-section>

            <q-separator></q-separator>

            <q-card-section>
                <div class="q-pt-md">
                    <q-markup-table flat separator="cell">
                        <tbody>
                            <tr>
                                <td class="text-right">API名称</td>
                                <td class="text-left">
                                    <q-input
                                        outlined
                                        round
                                        dense
                                        hide-bottom-space
                                        v-model="inputWhoisApiData.apiName"
                                        lazy-rules
                                        :rules="[$rules.required('请设置API名称')]"
                                    />
                                </td>
                            </tr>
                            <tr>
                                <td class="text-right">API URL</td>
                                <td class="text-left">
                                    <q-input
                                        outlined
                                        round
                                        dense
                                        hide-bottom-space
                                        v-model="inputWhoisApiData.apiUrl"
                                        type="textarea"
                                        placeholder="填写新地址，{domain} 为域名占位符；已有接口留空保持原地址，重命名需重填"
                                        lazy-rules
                                        :rules="[]"
                                    />
                                </td>
                            </tr>
                            <tr>
                                <td class="text-right">Free标识字符</td>
                                <td class="text-left">
                                    <div
                                        v-for="(textItem, index) in inputWhoisApiData.freeText"
                                        :key="index"
                                        class="q-gutter-md row items-center q-py-xs"
                                    >
                                        <q-chip color="info" text-color="white" size="sm">{{ index + 1 }}</q-chip>
                                        <q-input
                                            outlined
                                            round
                                            dense
                                            hide-bottom-space
                                            v-model="inputWhoisApiData.freeText[index]"
                                            lazy-rules
                                            :rules="[$rules.required('请设置Free标识字符')]"
                                        />
                                        <q-btn round flat color="negative" size="xs" icon="fa-solid fa-x" @click="removeFreeText(index)" />
                                    </div>
                                    <div class="q-pt-sm">
                                        <q-btn size="sm" color="primary" label="添加" @click="addFreeText" class="items-center q-px-lg" />
                                    </div>
                                </td>
                            </tr>
                            <tr>
                                <td class="text-right">Taken标识字符</td>
                                <td class="text-left">
                                    <div
                                        v-for="(textItem, index) in inputWhoisApiData.takenText"
                                        :key="index"
                                        class="q-gutter-md row items-center q-py-xs"
                                    >
                                        <q-chip color="info" text-color="white" size="sm">{{ index + 1 }}</q-chip>
                                        <q-input
                                            outlined
                                            round
                                            dense
                                            hide-bottom-space
                                            v-model="inputWhoisApiData.takenText[index]"
                                            lazy-rules
                                            :rules="[$rules.required('请设置Taken标识字符')]"
                                        />
                                        <q-btn round flat color="negative" size="xs" icon="fa-solid fa-x" @click="removeTakenText(index)" />
                                    </div>
                                    <div class="q-pt-sm">
                                        <q-btn size="sm" color="primary" label="添加" @click="addTakenText" class="items-center q-px-lg" />
                                    </div>
                                </td>
                            </tr>
                            <tr>
                                <td class="text-right">并发数限制</td>
                                <td class="text-left">
                                    <q-input
                                        type="number"
                                        outlined
                                        round
                                        dense
                                        hide-bottom-space
                                        v-model.number="inputWhoisApiData.concurrencyLimit"
                                        lazy-rules
                                        :rules="[
                                            $rules.required('请设置并发限制'),
                                            $rules.numeric('并发限制必须为数字'),
                                            $rules.minValue(1, '并发限制最少为1')
                                        ]"
                                    />
                                </td>
                            </tr>
                        </tbody>
                    </q-markup-table>
                </div>
            </q-card-section>

            <q-separator></q-separator>

            <q-card-actions align="right">
                <q-btn color="info" label="取消" v-close-popup />
                <q-btn color="green" label="修改" @click="updateWhoisApi()" v-if="updateWhoisApiItem.apiName" />
                <q-btn color="green" label="添加" @click="addWhoisApi()" v-else />
            </q-card-actions>
        </q-card>
    </q-dialog>
</template>

<script setup scoped>
defineOptions({
    name: "AdminSetting"
});

import { ref, onMounted } from "vue";
import { useQuasar, is, date } from "quasar";
import { api } from "boot/axios";
import { useSettingStore } from "src/stores/settingStore";

const $q = useQuasar();
const protocolFields = [
    { key: 'rdapTimeout', label: 'RDAP 超时 / 秒', max: 120 },
    ...[['dns', 'DNS'], ['whois', 'WHOIS'], ['rdap', 'RDAP']].flatMap(([key, label]) => [
        { key: `${key}Concurrency`, label: `${label} 最大并发`, max: 256 },
        { key: `${key}Rate`, label: `${label} 每上游每秒请求数`, max: 256 }
    ])
];

const settingStore = useSettingStore();

const settingForm = ref(null);

const settings = ref(null);
const loadFailed = ref(false);
const logLevelOptions = ["Debug", "Info", "Warn", "Error", "Off"];

const submitting = ref(false);
const downloadingLog = ref(false);

const ccTldOptions = ref([]);
const selectedCcTlds = ref([]);
const inputCcTld = ref("");

const displayRegisterApiEditDialog = ref(false);
const displayWhoisApiEditDialog = ref(false);

const updateRegisterApiItem = ref({
    apiName: "",
    apiUrl: "",
    successText: [],
    failText: [],
    concurrencyLimit: 1
});
const updateWhoisApiItem = ref({
    apiName: "",
    apiUrl: "",
    freeText: [],
    takenText: [],
    concurrencyLimit: 1
});

const inputRegisterApiData = ref({
    apiName: "",
    apiUrl: "",
    successText: [],
    failText: [],
    concurrencyLimit: 1
});
const inputWhoisApiData = ref({
    apiName: "",
    apiUrl: "",
    freeText: [],
    takenText: [],
    concurrencyLimit: 1
});

// 保留的Whois API接口名称
const reservedWhoisApiNames = ref(["whois", "rdap", "whoisQuery", "whoisQueryWithProxy", "dnsQuery", "mixedQuery"]);

function getSettings() {
    loadFailed.value = false;
    api.get("/admin/setting")
        .then((response) => {
            if (response.data) {
                settings.value = response.data;
                settings.value.secretActions = {authPassword:'keep',jwtSecretKey:'keep',socketProxyPassword:'keep'};
                if (settings.value.globalProxyTlds) {
                    settings.value.globalProxyTldsInput = settings.value.globalProxyTlds.join("\n");
                } else {
                    settings.value.globalProxyTldsInput = "";
                }

                if (settings.value.mixedProxyTlds) {
                    settings.value.mixedProxyTldsInput = settings.value.mixedProxyTlds.join("\n");
                } else {
                    settings.value.mixedProxyTldsInput = "";
                }

                if (settings.value.mixedDnsTlds) {
                    settings.value.mixedDnsTldsInput = settings.value.mixedDnsTlds.join("\n");
                } else {
                    settings.value.mixedDnsTldsInput = "";
                }

                if (settings.value.typoDefaultCcTlds) {
                    selectedCcTlds.value = [];
                    ccTldOptions.value = [];
                    settings.value.typoDefaultCcTlds.forEach((tld) => {
                        if (tld.isSelected) {
                            selectedCcTlds.value.push(tld.tld);
                        }
                        ccTldOptions.value.push(tld.tld);
                    });
                } else {
                    selectedCcTlds.value = [];
                    ccTldOptions.value = [];
                }

                if (settings.value.typoCustomizedReplaces) {
                    settings.value.typoCustomizedReplacesInput = settings.value.typoCustomizedReplaces.join("\n");
                } else {
                    settings.value.typoCustomizedReplacesInput = "";
                }
            } else {
                loadFailed.value = true;
            }
        })
        .catch(() => {
            loadFailed.value = true;
        });
}

function showValidationError() {
    $q.notify({
        position: "top",
        type: "negative",
        message: "参数校验失败, 请检查参数输入"
    });
}

function resetLog() {
    $q.dialog({
        message: "是否确认要清空当前日志?",
        title: " ",
        cancel: "取消",
        ok: "确定",
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
            .catch((error) => {
                $q.notify({
                    position: "top",
                    type: "negative",
                    message: "清空日志失败"
                });
            });
    });
}

function downloadLog() {
    const defaultFilename = "log_" + date.formatDate(Date.now(), "YYYY-MM-DD_HH-mm-ss") + ".zip";

    downloadingLog.value = true;

    api.get("/admin/log", {
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

            downloadingLog.value = false;
        })
        .catch((error) => {
            $q.notify({
                position: "top",
                type: "negative",
                message: "下载日志文件失败, 请稍后重试"
            });

            downloadingLog.value = false;
        });
}

function addCcTld() {
    if (inputCcTld.value) {
        let newTld = inputCcTld.value.replace(/\s+/g, "").replace(/^\.+/, "").replace(/\.+$/, "").toLowerCase();
        if (newTld.length > 0) {
            if (ccTldOptions.value.includes(newTld)) {
                $q.notify({
                    position: "top",
                    type: "warning",
                    message: "TLD已存在"
                });
            } else {
                ccTldOptions.value.push(newTld);
                inputCcTld.value = "";
            }
        } else {
            $q.notify({
                position: "top",
                type: "warning",
                message: "输入的TLD无效"
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
        const lines = value.split("\n");
        for (const line of lines) {
            const [key, value] = line.split(":").map((item) => item.trim());
            if (!key || !value) {
                return "请输入正确的自定义Typo替换词, 格式: 原字符:替换字符";
            }
        }
    }
    return true;
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
        successText: [],
        failText: [],
        concurrencyLimit: 1
    };
    inputRegisterApiData.value = {
        apiName: "",
        apiUrl: "",
        successText: [],
        failText: [],
        concurrencyLimit: 1
    };
}

function addRegisterApi() {
    if (!settings.value.registerApis) {
        settings.value.registerApis = [];
    }

    if (settings.value.registerApis.find((item) => item.apiName == inputRegisterApiData.value.apiName)) {
        $q.notify({
            position: "top",
            type: "negative",
            message: "注册接口名称已存在, 请使用其他名称"
        });
        return;
    }
    settings.value.registerApis.push({
        apiName: inputRegisterApiData.value.apiName,
        apiUrl: inputRegisterApiData.value.apiUrl,
        successText: inputRegisterApiData.value.successText,
        failText: inputRegisterApiData.value.failText,
        concurrencyLimit: inputRegisterApiData.value.concurrencyLimit
    });
    displayRegisterApiEditDialog.value = false;
}

function openUpdateRegisterApiDialog(apiItem) {
    displayRegisterApiEditDialog.value = true;
    updateRegisterApiItem.value = apiItem;
    inputRegisterApiData.value = {
        apiName: apiItem.apiName,
        apiUrl: apiItem.apiUrl,
        successText: [...apiItem.successText],
        failText: [...apiItem.failText],
        concurrencyLimit: apiItem.concurrencyLimit
    };
}

function updateRegisterApi() {
    // 检查是否有重名的API,排除当前正在编辑的API
    if (
        settings.value.registerApis.find(
            (item) => item.apiName == inputRegisterApiData.value.apiName && item.apiName != updateRegisterApiItem.value.apiName
        )
    ) {
        $q.notify({
            position: "top",
            type: "negative",
            message: "注册接口名称已存在, 请使用其他名称"
        });
        return;
    }

    const targetApi = settings.value.registerApis.find((item) => item.apiName == updateRegisterApiItem.value.apiName);

    if (targetApi) {
        // 更新API名称和URL
        targetApi.apiName = inputRegisterApiData.value.apiName;
        targetApi.apiUrl = inputRegisterApiData.value.apiUrl;
        targetApi.successText = inputRegisterApiData.value.successText;
        targetApi.failText = inputRegisterApiData.value.failText;
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
        freeText: [],
        takenText: [],
        concurrencyLimit: 1
    };
    inputWhoisApiData.value = {
        apiName: "",
        apiUrl: "",
        freeText: [],
        takenText: [],
        concurrencyLimit: 1
    };
}

function addWhoisApi() {
    if (!settings.value.whoisApis) {
        settings.value.whoisApis = [];
    }

    if (reservedWhoisApiNames.value.includes(inputWhoisApiData.value.apiName)) {
        $q.notify({
            position: "top",
            type: "negative",
            message: `${inputWhoisApiData.value.apiName} 是系统保留的接口名称, 请使用其他名称`
        });
        return;
    }

    if (settings.value.whoisApis.find((item) => item.apiName == inputWhoisApiData.value.apiName)) {
        $q.notify({
            position: "top",
            type: "negative",
            message: "Whois接口名称已存在, 请使用其他名称"
        });
        return;
    }
    settings.value.whoisApis.push({
        apiName: inputWhoisApiData.value.apiName,
        apiUrl: inputWhoisApiData.value.apiUrl,
        freeText: inputWhoisApiData.value.freeText,
        takenText: inputWhoisApiData.value.takenText,
        concurrencyLimit: inputWhoisApiData.value.concurrencyLimit
    });
    displayWhoisApiEditDialog.value = false;
}

function openUpdateWhoisApiDialog(apiItem) {
    displayWhoisApiEditDialog.value = true;
    updateWhoisApiItem.value = apiItem;
    inputWhoisApiData.value = {
        apiName: apiItem.apiName,
        apiUrl: apiItem.apiUrl,
        freeText: [...apiItem.freeText],
        takenText: [...apiItem.takenText],
        concurrencyLimit: apiItem.concurrencyLimit
    };
}

function updateWhoisApi() {
    if (reservedWhoisApiNames.value.includes(inputWhoisApiData.value.apiName)) {
        $q.notify({
            position: "top",
            type: "negative",
            message: `${inputWhoisApiData.value.apiName} 是系统保留的接口名称, 请使用其他名称`
        });
        return;
    }

    // 检查是否有重名的API,排除当前正在编辑的API
    if (
        settings.value.whoisApis.find((item) => item.apiName == inputWhoisApiData.value.apiName && item.apiName != updateWhoisApiItem.value.apiName)
    ) {
        $q.notify({
            position: "top",
            type: "negative",
            message: "Whois接口名称已存在, 请使用其他名称"
        });
        return;
    }

    const targetApi = settings.value.whoisApis.find((item) => item.apiName == updateWhoisApiItem.value.apiName);

    if (targetApi) {
        // 更新API名称和URL
        targetApi.apiName = inputWhoisApiData.value.apiName;
        targetApi.apiUrl = inputWhoisApiData.value.apiUrl;
        targetApi.freeText = inputWhoisApiData.value.freeText;
        targetApi.takenText = inputWhoisApiData.value.takenText;
        targetApi.concurrencyLimit = inputWhoisApiData.value.concurrencyLimit;
    }
    displayWhoisApiEditDialog.value = false;
}

function deleteWhoisApi(apiItem) {
    settings.value.whoisApis = settings.value.whoisApis.filter((item) => item.apiName != apiItem.apiName);
}

function onSubmit() {
    settingForm.value.validate().then((success) => {
        if (success) {
            submitting.value = true;

            if (!settings.value.retryOnTimeout) {
                if (!is.number(settings.value.retryInterval)) {
                    settings.value.retryInterval = 0;
                }
                if (!is.number(settings.value.retryMax)) {
                    settings.value.retryMax = 0;
                }
            }

            if (settings.value.globalProxyTldsInput) {
                settings.value.globalProxyTlds = settings.value.globalProxyTldsInput.split("\n");
            } else {
                settings.value.globalProxyTlds = [];
            }

            if (settings.value.mixedProxyTldsInput) {
                settings.value.mixedProxyTlds = settings.value.mixedProxyTldsInput.split("\n");
            } else {
                settings.value.mixedProxyTlds = [];
            }

            if (settings.value.mixedDnsTldsInput) {
                settings.value.mixedDnsTlds = settings.value.mixedDnsTldsInput.split("\n");
            } else {
                settings.value.mixedDnsTlds = [];
            }

            if (ccTldOptions.value.length > 0) {
                settings.value.typoDefaultCcTlds = [];
                ccTldOptions.value.forEach((tld) => {
                    settings.value.typoDefaultCcTlds.push({
                        tld: tld,
                        isSelected: selectedCcTlds.value.includes(tld)
                    });
                });
            } else {
                settings.value.typoDefaultCcTlds = [];
            }

            if (settings.value.typoCustomizedReplacesInput) {
                settings.value.typoCustomizedReplaces = settings.value.typoCustomizedReplacesInput.split("\n");
            } else {
                settings.value.typoCustomizedReplaces = [];
            }

            api.put("/admin/setting", { ...settings.value, socketProxyPort: settings.value.socketProxyPort || 0 })
                .then((response) => {
                    $q.notify({
                        position: "top",
                        type: "positive",
                        message: "配置参数保存成功"
                    });
                    submitting.value = false;
                    settings.value.authPassword = '';
                    settings.value.socketProxyPassword = '';
                    settings.value.jwtSecretKey = '';
                    settings.value.secretActions = {authPassword:'keep',jwtSecretKey:'keep',socketProxyPassword:'keep'};
                    settings.value.secrets = response.data.secrets;
                    for (const key of ['whoisApis','registerApis']) settings.value[key] = response.data[key];
                    settingStore.updateSetting(response.data);
                })
                .catch((error) => {
                    $q.notify({
                        position: "top",
                        type: "negative",
                        message: error.response?.status === 400 && typeof error.response.data === "string"
                            ? "保存失败：" + error.response.data
                            : "配置保存失败，请检查连接后重试"
                    });
                    submitting.value = false;
                });
        } else {
            $q.notify({
                position: "top",
                type: "negative",
                message: "参数校验失败, 请检查参数输入"
            });
        }
    });
}

onMounted(() => {
    getSettings();
});
</script>

<style scoped>
.api-editor {
    width: 36rem;
    max-width: 94vw;
}
@media (max-width: 600px) {
    .api-editor tr {
        display: grid;
    }
    .api-editor td {
        height: auto;
        border: 0;
        padding: 4px 8px 8px;
        text-align: left;
        white-space: normal;
    }
    .api-editor td:first-child {
        padding-top: 12px;
        font-weight: 600;
    }
}
</style>
