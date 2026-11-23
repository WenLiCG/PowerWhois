# Typonamer API 文档

本文档提供了 Typonamer 应用的 API 接口说明，包括 HTTP API 和 WebSocket API。

## 目录

- [HTTP API](#http-api)
  - [认证相关](#认证相关)
  - [配置相关](#配置相关)
  - [日志相关](#日志相关)
  - [批量检查相关](#批量检查相关)
  - [数据结构](#http-api-数据结构)
- [WebSocket API](#websocket-api)
  - [连接建立](#连接建立)
  - [事件类型](#事件类型)
  - [管理员认证](#管理员认证)
  - [批量检查操作](#批量检查操作)
  - [网页检查操作](#网页检查操作)
  - [typo 检查操作](#typo检查操作)
  - [域名注册操作](#域名注册操作)
  - [数据结构](#websocket-api-数据结构)
- [公共状态和常量](#公共状态和常量)

## HTTP API

所有 HTTP API 路由以`/api`为前缀。

### 认证相关

#### 登录

```
POST /api/login
```

用于用户身份验证并生成 JWT 令牌。

**请求体**：

```json
{
  "username": "管理员用户名", // string: 用户名
  "password": "管理员密码" // string: 密码
}
```

**响应**：

- 成功 (200)
  ```json
  {
    "username": "管理员用户名", // string: 用户名
    "token": "JWT令牌" // string: JWT令牌字符串
  }
  ```
- 请求无效 (400)：用户名或密码不正确
- 服务器错误 (500)：令牌生成失败

### 配置相关

| 接口           | 方法 | 路径               | 描述                       | 需要认证 |
| -------------- | ---- | ------------------ | -------------------------- | -------- |
| 获取网页配置   | GET  | /api/web/setting   | 获取用于网页前端的配置信息 | 否       |
| 获取管理员配置 | GET  | /api/admin/setting | 获取管理员配置信息         | 是       |
| 更新配置       | PUT  | /api/admin/setting | 更新系统配置               | 是       |

#### 获取公共网页配置

**响应** (200)：

```json
{
  "webCheckDomainLimit": 500, // int: 网页查询域名数量限制
  "typoDefaultCcTlds": ["com", "net", "org", "com.cn"], // string[]: 默认选中的顶级域名列表
  "registerApis": ["test"], // string[]: 注册API列表
  "whoisApis": [] // string[]: Whois API列表
}
```

#### 获取管理员配置

**请求头**：

- `Authorization`: Bearer {JWT 令牌}

**响应** (200)：完整配置信息

```json
{
  "logLevel": "Info", // string: 日志等级，可选值：Error, Warn, Info, Debug, Off
  "authUsername": "admin", // string: 认证用户名
  "authPassword": "123456", // string: 认证密码
  "authExpireDays": 30, // int: 认证过期天数
  "whoisTimeout": 5, // int: whois查询超时时间(秒)
  "dnsTimeout": 3, // int: DNS查询超时时间(秒)
  "retryOnTimeout": true, // bool: 超时时是否重试
  "retryInterval": 3, // int: 重试间隔时间(秒)
  "retryMax": 3, // int: 最大重试次数
  "globalProxyTlds": ["co", "hk", "tw", "au", "us"], // string[]: 强制使用代理的顶级域名
  "mixedProxyTlds": ["net"], // string[]: 混合查询中强制使用代理的顶级域名
  "mixedDnsTlds": ["hk"], // string[]: 混合查询中强制使用DNS检查的顶级域名
  "socketProxyHost": "192.168.1.1", // string: 代理服务器地址
  "socketProxyPort": 1080, // int: 代理服务器端口
  "socketProxyAuth": false, // bool: 代理服务器是否需要认证
  "socketProxyUser": "test", // string: 代理服务器用户名
  "socketProxyPassword": "123456", // string: 代理服务器密码
  "bulkCheckConcurrencyLimit": 100, // int: 批量检查并发限制
  "webCheckConcurrencyLimit": 10, // int: 网页检查并发限制
  "webCheckDomainLimit": 500, // int: 网页查询域名数量限制
  "typoDefaultCcTlds": [
    // array: 默认顶级域名列表
    {
      "tld": "com", // string: 顶级域名
      "isSelected": true // bool: 是否默认选中
    },
    {
      "tld": "net",
      "isSelected": true
    },
    {
      "tld": "org",
      "isSelected": true
    },
    {
      "tld": "co",
      "isSelected": false
    },
    {
      "tld": "de",
      "isSelected": false
    },
    {
      "tld": "eu",
      "isSelected": false
    },
    {
      "tld": "in",
      "isSelected": false
    },
    {
      "tld": "com.cn",
      "isSelected": true
    }
  ],
  "typoCustomizedReplaces": [], // array: 自定义替换规则
  "registerApis": [
    // array: 注册API配置
    {
      "apiName": "test", // string: API名称
      "apiUrl": "http://192.168.1.1:8080/{domain}", // string: API地址
      "successText": ["ok"], // string[]: 成功响应文本
      "failText": ["failed"], // string[]: 失败响应文本
      "concurrencyLimit": 1 // int: 并发限制
    }
  ],
  "whoisApis": [
    // array: Whois API配置
    {
      "apiName": "test", // string: API名称
      "apiUrl": "http://192.168.1.1:8080/{domain}", // string: API地址
      "FreeText": ["free"], // string[]: 未注册匹配文本
      "TakenText": ["taken"], // string[]: 已注册匹配文本
      "concurrencyLimit": 1 // int: 并发限制
    }
  ]
}
```

#### 更新配置

**请求头**：

- `Authorization`: Bearer {JWT 令牌}

**请求体**：
新的配置信息

**响应**：

- 成功 (200)：更新后的配置信息
- 失败 (500)：错误信息

### 日志相关

| 接口     | 方法   | 路径           | 描述         | 需要认证 |
| -------- | ------ | -------------- | ------------ | -------- |
| 下载日志 | GET    | /api/admin/log | 下载系统日志 | 是       |
| 重置日志 | DELETE | /api/admin/log | 清空系统日志 | 是       |

#### 下载日志

**请求头**：

- `Authorization`: Bearer {JWT 令牌}

**响应**：

- 成功 (200)：日志文件内容（二进制数据流，ZIP 格式）
- 失败 (500)：错误信息

#### 重置日志

**请求头**：

- `Authorization`: Bearer {JWT 令牌}

**响应**：

- 成功 (200)
- 失败 (500)：错误信息

### 批量检查相关

| 接口             | 方法 | 路径                               | 描述                     | 需要认证 |
| ---------------- | ---- | ---------------------------------- | ------------------------ | -------- |
| 批量域名上传     | POST | /api/admin/bulkcheckupload         | 上传批量域名文件用于检查 | 是       |
| 批量检查结果下载 | GET  | /api/admin/bulkcheckresultdownload | 下载批量域名检查的结果   | 是       |

#### 批量域名上传

**请求头**：

- `Authorization`: Bearer {JWT 令牌}
- `Content-Type`: multipart/form-data

**表单字段**：

- `file`: 包含域名列表的文件（文本文件，每行一个域名）

**响应**：

- 成功 (200)
- 失败 (500)：错误信息

#### 批量检查结果下载

**请求头**：

- `Authorization`: Bearer {JWT 令牌}

**响应**：

- 成功 (200)：CSV 格式的检查结果
- 失败 (500)：错误信息

### HTTP API 数据结构

#### 登录信息 (LoginInfo)

```json
{
  "username": "string", // 用户名
  "password": "string" // 密码
}
```

#### 配置信息 (Config)

```json
{
  "logLevel": "Info", // string: 日志等级，可选值：Error, Warn, Info, Debug, Off
  "authUsername": "admin", // string: 认证用户名
  "authPassword": "123456", // string: 认证密码
  "authExpireDays": 30, // int: 认证过期天数
  "whoisTimeout": 5, // int: whois查询超时时间(秒)
  "dnsTimeout": 3, // int: DNS查询超时时间(秒)
  "retryOnTimeout": true, // bool: 超时时是否重试
  "retryInterval": 3, // int: 重试间隔时间(秒)
  "retryMax": 3, // int: 最大重试次数
  "globalProxyTlds": ["co", "hk", "tw", "au", "us"], // string[]: 强制使用代理的顶级域名
  "mixedProxyTlds": ["net"], // string[]: 混合查询中强制使用代理的顶级域名
  "mixedDnsTlds": ["hk"], // string[]: 混合查询中强制使用DNS检查的顶级域名
  "socketProxyHost": "192.168.1.1", // string: 代理服务器地址
  "socketProxyPort": 1080, // int: 代理服务器端口
  "socketProxyAuth": false, // bool: 代理服务器是否需要认证
  "socketProxyUser": "test", // string: 代理服务器用户名
  "socketProxyPassword": "123456", // string: 代理服务器密码
  "bulkCheckConcurrencyLimit": 100, // int: 批量检查并发限制
  "webCheckConcurrencyLimit": 10, // int: 网页检查并发限制
  "webCheckDomainLimit": 500, // int: 网页查询域名数量限制
  "typoDefaultCcTlds": [
    // array: 默认顶级域名列表
    {
      "tld": "com", // string: 顶级域名
      "isSelected": true // bool: 是否默认选中
    },
    {
      "tld": "net",
      "isSelected": true
    },
    {
      "tld": "org",
      "isSelected": true
    },
    {
      "tld": "co",
      "isSelected": false
    },
    {
      "tld": "de",
      "isSelected": false
    },
    {
      "tld": "eu",
      "isSelected": false
    },
    {
      "tld": "in",
      "isSelected": false
    },
    {
      "tld": "com.cn",
      "isSelected": true
    }
  ],
  "typoCustomizedReplaces": [], // array: 自定义替换规则
  "registerApis": [
    // array: 注册API配置
    {
      "apiName": "test", // string: API名称
      "apiUrl": "http://192.168.1.1:8080/{domain}", // string: API地址
      "successText": ["ok"], // string[]: 成功响应文本
      "failText": ["failed"], // string[]: 失败响应文本
      "concurrencyLimit": 1 // int: 并发限制
    }
  ],
  "whoisApis": [
    // array: Whois API配置
    {
      "apiName": "test", // string: API名称
      "apiUrl": "http://192.168.1.1:8080/{domain}", // string: API地址
      "FreeText": ["free"], // string[]: 未注册匹配文本
      "TakenText": ["taken"], // string[]: 已注册匹配文本
      "concurrencyLimit": 1 // int: 并发限制
    }
  ]
}
```

## WebSocket API

WebSocket 连接地址：

- 公共连接地址：`/app/ws`
- 管理员连接地址：`/app/ws?token={JWT 令牌}`

### 连接建立

WebSocket 连接建立后，客户端和服务器可以使用消息事件进行通信。消息格式如下：

```json
{
  "event": "事件名称",  // string: 事件名称
  "data": 事件数据     // 任意类型: 事件数据，根据事件类型不同而不同
}
```

### 事件类型

#### 请求事件

| 事件名称                  | 描述             | 需要认证 |
| ------------------------- | ---------------- | -------- |
| `ping`                    | 心跳检测         | 否       |
| `adminAuth`               | 管理员认证       | 否       |
| `bulkCheckStart`          | 开始批量检查     | 是       |
| `bulkCheckPause`          | 暂停批量检查     | 是       |
| `bulkCheckResume`         | 恢复批量检查     | 是       |
| `bulkCheckCancel`         | 取消批量检查     | 是       |
| `bulkCheckClear`          | 清除批量检查     | 是       |
| `bulkRecheckErrorDomains` | 重新检查错误域名 | 是       |
| `webCheck`                | 网页检查         | 否       |
| `typoCheck`               | Typo 检查        | 是       |
| `register`                | 域名注册         | 是       |

#### 响应事件

| 事件名称          | 描述             |
| ----------------- | ---------------- |
| `pong`            | 心跳响应         |
| `webCheckDomains` | 网页检查域名列表 |
| `webCheckResult`  | 网页检查结果     |
| `typoResult`      | Typo 检查结果    |
| `registerResult`  | 域名注册结果     |
| `bulkCheckError`  | 批量检查错误     |
| `bulkCheckInfo`   | 批量检查信息     |
| `webCheckError`   | 网页检查错误     |
| `typoCheckError`  | Typo 检查错误    |
| `registerError`   | 域名注册错误     |

### 管理员认证

**请求**：

```json
{
  "event": "adminAuth",
  "data": "JWT令牌" // string: JWT令牌字符串
}
```

**响应**：无显式响应，认证后可使用管理员功能

### 批量检查操作

#### 开始批量检查

**请求**：

```json
{
  "event": "bulkCheckStart",
  "data": {
    "queryType": "查询类型" // string: 查询类型，可选值见下方说明
  }
}
```

查询类型可以是：

- `whoisQuery`: 不使用代理的 whois 查询
- `whoisQueryWithProxy`: 使用代理的 whois 查询
- `dnsQuery`: DNS 查询
- `mixedQuery`: 混合查询
- 在后台已自定义的 Whois 查询接口名称

**响应**：通过`bulkCheckInfo`事件返回批量检查状态

```json
{
  "event": "bulkCheckInfo",
  "data": {
    "Status": "状态", // string: 批量检查状态
    "QueryType": "类型", // string: 批量查询类型
    "TotalDomains": 0, // int: 去重域名数量
    "RemainDomains": 0, // int: 未检查域名数量
    "TakenDomains": 0, // int: 已注册域名数量
    "FreeDomains": 0, // int: 可注册域名数量
    "ErrorDomains": 0 // int: 错误域名数量
  }
}
```

#### 暂停批量检查

**请求**：

```json
{
  "event": "bulkCheckPause",
  "data": null
}
```

**响应**：通过`bulkCheckInfo`事件返回状态为`paused`的批量检查状态

#### 恢复批量检查

**请求**：

```json
{
  "event": "bulkCheckResume",
  "data": null
}
```

**响应**：通过`bulkCheckInfo`事件返回状态为`running`的批量检查状态

#### 取消批量检查

**请求**：

```json
{
  "event": "bulkCheckCancel",
  "data": null
}
```

**响应**：通过`bulkCheckInfo`事件返回状态为`canceled`的批量检查状态

#### 清除批量检查

**请求**：

```json
{
  "event": "bulkCheckClear",
  "data": null
}
```

**响应**：通过`bulkCheckInfo`事件返回状态为`idle`的批量检查状态

#### 重新检查错误域名

**请求**：

```json
{
  "event": "bulkRecheckErrorDomains",
  "data": null
}
```

**响应**：通过`bulkCheckInfo`事件返回更新的批量检查状态

### 网页检查操作

**请求**：

```json
{
  "event": "webCheck",
  "data": {
    "queryType": "查询类型", // string: 查询类型，同批量检查查询类型
    "domains": ["域名1", "域名2"] // string[]: 域名列表
  }
}
```

**响应**：通过`webCheckResult`事件返回查询结果

```json
{
  "event": "webCheckResult",
  "data": {
    "order": 0, // 查询顺序
    "domain": "string", // 域名
    "lookupType": "string", // 查询类型
    "viaProxy": false, // 是否使用代理
    "queryError": "string", // 查询错误信息
    "registerStatus": "string", // 注册状态
    "createdDate": "string", // 创建日期
    "expiryDate": "string", // 过期日期
    "nameServer": ["string"], // 域名服务器列表
    "dnsLite": "string", // DNS精简信息
    "rawDomainStatus": ["string"], // 原始域名状态列表
    "domainStatus": "string", // 域名状态
    "rawResponse": "string" // 原始响应内容
  }
}
```

### typo 检查操作

**请求**：

```json
{
  "event": "typoCheck",
  "data": {
    "domain": "域名", // string: 要检查的域名
    "typoType": ["typo类型1", "typo类型2"], // string[]: typo类型列表
    "ccTlds": ["com", "net"], // string[]: 要检查的顶级域名列表
    "queryType": "查询类型" // string: 查询类型，同批量检查查询类型
  }
}
```

typo 类型可以是：

- `www`: WWW 前缀 typo
- `skipLetter`: 跳过字母 typo
- `doubleLetter`: 双字母 typo
- `reverseLetter`: 反转字母 typo
- `insertedLetter`: 插入字母 typo
- `wrongHorizontalKey`: 错误水平键 typo
- `wrongVerticalKey`: 错误垂直键 typo
- `wrongTlds`: 错误 TLD typo
- `customizedReplace`: 自定义替换 typo

**响应**：通过`typoResult`事件返回 typo 检查结果

```json
{
  "event": "typoResult",
  "data": {
    "typoType": "typo类型", // string: typo类型
    "domains": ["域名1", "域名2"] // string[]: 生成的typo域名列表
  }
}
```

### 域名注册操作

**请求**：

```json
{
  "event": "register",
  "data": {
    "registerType": "注册类型", // string: 注册类型
    "domains": ["域名1", "域名2"] // string[]: 要注册的域名列表
  }
}
```

**响应**：通过`registerResult`事件返回注册结果

```json
{
  "event": "registerResult",
  "data": {
    "registerType": "注册类型", // string: 注册类型
    "domainName": "域名", // string: 域名
    "registerStatus": "注册状态", // string: 注册状态，可选值：success, failed, error
    "rawResponse": "原始响应" // string: 原始响应内容
  }
}
```

### WebSocket API 数据结构

#### 消息对象 (MessageObject)

```json
{
  "event": "string", // 事件名称
  "data": "json" // 事件数据，JSON格式
}
```

#### Web 检查请求 (WebCheck)

```json
{
  "queryType": "string", // 查询类型
  "domains": ["string"] // 域名列表
}
```

#### Web 检查结果 (WebCheckResult)

```json
{
  "event": "webCheckResult",
  "data": {
    "order": 0, // 查询顺序
    "domain": "string", // 域名
    "lookupType": "string", // 查询类型
    "viaProxy": false, // 是否使用代理
    "queryError": "string", // 查询错误信息
    "registerStatus": "string", // 注册状态
    "createdDate": "string", // 创建日期
    "expiryDate": "string", // 过期日期
    "nameServer": ["string"], // 域名服务器列表
    "dnsLite": "string", // DNS精简信息
    "rawDomainStatus": ["string"], // 原始域名状态列表
    "domainStatus": "string", // 域名状态
    "rawResponse": "string" // 原始响应内容
  }
}
```

#### Typo 检查请求 (TypoCheck)

```json
{
  "domain": "string", // 域名
  "typoType": ["string"], // typo类型列表
  "ccTlds": ["string"], // 顶级域名列表
  "queryType": "string" // 查询类型
}
```

#### Typo 检查结果 (TypoResult)

```json
{
  "typoType": "typo类型", // string: typo类型
  "domains": ["域名1", "域名2"] // string[]: 生成的typo域名列表
}
```

#### 注册请求 (Register)

```json
{
  "registerType": "string", // 注册类型
  "domains": ["string"] // 域名列表
}
```

#### 注册结果 (RegisterResult)

```json
{
  "registerType": "string", // 注册类型
  "domainName": "string", // 域名
  "registerStatus": "string", // 注册状态，可选值：success, failed, error
  "rawResponse": "string" // 原始响应内容
}
```

#### 批量检查信息 (BulkCheckInfo)

```json
{
  "Status": "string", // 批量检查状态
  "QueryType": "string", // 查询类型
  "TotalDomains": 0, // 总域名数量
  "RemainDomains": 0, // 剩余未检查域名数量
  "TakenDomains": 0, // 已注册域名数量
  "FreeDomains": 0, // 可注册域名数量
  "ErrorDomains": 0 // 错误域名数量
}
```

## 公共状态和常量

### 域名注册状态

- `Taken`: 已被注册
- `Free`: 可注册
- `Error`: 查询错误

### 域名状态

- `Active`: 激活状态
- `Expired`: 已过期
- `RedemptionPeriod`: 赎回期
- `PendingDelete`: 待删除
- `Unknown`: 未知状态

### 注册操作状态

- `success`: 注册成功
- `failed`: 注册失败
- `error`: 注册错误

### 批量检查状态

- `idle`: 空闲状态
- `init`: 初始化
- `uniquing`: 正在进行域名去重
- `running`: 运行中
- `paused`: 已暂停
- `done`: 已完成
- `canceled`: 已取消
- `error`: 错误

### 查询类型

- `whoisQuery`: 不使用代理的 whois 查询
- `whoisQueryWithProxy`: 使用代理的 whois 查询
- `dnsQuery`: DNS 查询
- `mixedQuery`: 混合查询
- 后台定义的查询接口
