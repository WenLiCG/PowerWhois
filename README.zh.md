[English](./README.md) | 中文

# PowerWhois

## 网页版: [link](https://whois.1ka.net)

## screenshot
![screenshot1](./img/screen1.jpg)
![screenshot2](./img/screen2.jpg)
![screenshot3](./img/screen3.jpg)
![screenshot4](./img/screen4.jpg)
![screenshot5](./img/screen5.jpg)


## 功能介绍
1. 除了网页查询，支持批量查询域名，测试上传了100w域名没有问题，而且速度巨快
2. 网页查询的结果点击后会弹出原始whois信息
3. 支持所有的后缀，部分只能使用DNS模式，所以少部分被注册商保留的域名可能不准确，但应该极少
4. 支持设置代理，并可以针对不同的后缀设置代理
5. 新增Mix模式，此模式下针对不同的后缀可以自由设置直连，代理，或者DNS扫描
6. 支持typo生成
7. 支持自定义whois的API，可以把自己常用的注册商的API自己加进去查询域名的可用性
8. 支持域名注册，并且自定义register的API，可以把自己常用的注册商的API自己加进去，方便注册域名
9. 后台设置可以控制线程，失败重试等各种设置
10. 支持API模式，你可以把程序作为一个API使用，比如接入你的WHMCS，[详见说明](https://github.com/WenLiCG/PowerWhois/blob/main/API%20documentation.md)

## 目录结构

```
.
├── deploy  (部署相关)
│   ├── backend (后端程序)
│   ├── docker-compose.yml
│   ├── frontend    (前端程序)
│   ├── nginx       (Nginx配置文件)
│   └── redis       (Redis相关)
├── README.md   (说明文档)
└── src (源码目录)
    ├── backend     (后端源码)
    ├── frontend    (前端源码)
    └── Makefile
```

# 部署

## 服务器依赖

- docker

## 上传文件

将 deploy 文件夹中的内容原样上传到服务器中

## 配置

- 修改`deploy/backend/config.yaml`文件中的`AuthUsername`和`AuthPassword`。其中`AuthUsername`是管理员的用户名，`AuthPassword`是管理员密码。

- 将 Nginx 配置文件`deploy/nginx/typonamer.conf`中 443 端口和 80 端口的`server_name`修改为真实的目标域名。

- 将 Nginx 配置文件`deploy/nginx/typonamer.conf`中 443 端口的`ssl_certificate`路径和`ssl_certificate_key`路径修改为真实的目标证书路径和密钥路径。

- 将 Nginx 配置文件`deploy/nginx/typonamer.conf`中 443 端口的`root`路径修改为前端文件夹`deploy/frontend`在主机上的真实绝对路径。

- 将 Nginx 配置文件`deploy/nginx/typonamer.conf`复制到主机 Nginx 的配置目录中，主机的 Nginx 的配置目录一般为`/etc/nginx/conf.d/`
更新一个宝塔的配置文件,直接把`testdomain.net`换成你域名就可以
```
server {
    listen 443 ssl http2;

    server_name testdomain.net;  # 修改为真实的域名

    charset UTF-8;
    client_max_body_size 100M;
    keepalive_timeout    75s;
    keepalive_requests   100;

    ssl_certificate    /www/server/panel/vhost/cert/testdomain.net/fullchain.pem;
    ssl_certificate_key    /www/server/panel/vhost/cert/testdomain.net/privkey.pem;

    ssl_session_timeout 1d;
    ssl_session_cache shared:MozSSL:10m;
    ssl_session_tickets off;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384:DHE-RSA-CHACHA20-POLY1305;
    ssl_prefer_server_ciphers off;
    add_header Strict-Transport-Security "max-age=63072000" always;
    ssl_stapling on;
    ssl_stapling_verify on;

    location ~ ^/(api|app) {
        proxy_redirect off;
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-Ip $proxy_protocol_addr;
        proxy_set_header X-Forwarded-For $proxy_protocol_addr;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    location / {
        root /www/wwwroot/testdomain.net;  # 修改为前端文件夹路径
        try_files $uri $uri/ /index.html
        index index.html index.htm;
    }
}

server {
    listen       80;
    server_name  testdomain.net;  # 修改为真实的域名

    location / {
        return 301 https://$host$request_uri;
    }
}
```

## 部署

- 安装docker
- PowerWhois/deploy/frontend 内容复制到网站根目录
- cd 到服务器中的 deploy 目录
- 执行命令：`docker compose up -d`
- 重启主机上的 Nginx：`sudo systemctl restart nginx`
- 访问网站，例如：`https://typonamer.example.com`

# 开发

## 后端

后端程序是用 Golang 语言开发，Web 框架使用的是 Gofiber。

### 依赖

- Golang 1.23+

## 前端

前端程序使用 Vue3 开发，UI 框架使用的是 Quasar 2。

### 依赖

- Nodejs v20+
- yarn 1.22+

### 初始化

```
cd src/frontend
yarn
quasar dev
```

## 编译

项目源码目录中已编写好`src/Makefile`文件，在 Linux 中安装好后端依赖、前端依赖以及`make`命令之后即可开始编译。编译后的输出会自动替换 deploy 目录下 backend 和 frontend 中的内容，需要手动将编译后的新文件上传到服务器中替换旧的服务器文件，然后重启容器后生效。

### 单独编译后端代码

```
cd src/
make backend
```

### 单独编译前端代码

```
cd src/
make frontend
```

### 同时编译前端和后端代码

```
cd src/
make all
```

## 贡献

欢迎贡献！请随时提交 Pull Request。

## 为爱发电
[爱发电](https://afdian.com/a/Powerwhois)

