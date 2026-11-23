import { defineConfig } from "#q-app";
export default defineConfig((ctx) => ({
    boot: ["axios", "rules"],
    css: ["app.css"],
    extras: ["fontawesome-v6", "material-icons"],
    build: {
        vueRouterMode: "history",
        alias: { src: ctx.appPaths.srcDir, boot: ctx.appPaths.resolve.src("boot"), pages: ctx.appPaths.resolve.src("pages") }
    },
    devServer: {
        host: "127.0.0.1",
        port: 8090,
        open: false,
        proxy: {
            "/api": { target: "http://127.0.0.1:8080" },
            "/app/ws": { target: "ws://127.0.0.1:8080", ws: true }
        }
    },
    framework: { lang: "zh-CN", plugins: ["Notify", "Dialog"] }
}));
