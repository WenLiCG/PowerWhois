import { useWebSocket } from "@vueuse/core";
import { getToken } from "./tokenHandler";
import { useWebStore } from "src/stores/webStore";
import { Notify } from "quasar";

const wsUrl = new URL("/app/ws", window.location.href);
wsUrl.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

const webStore = useWebStore();

export const { status, send, open, close } = useWebSocket(wsUrl.href, {
    autoReconnect: true,
    heartbeat: {
        message: JSON.stringify({ event: "ping" }),
        responseMessage: JSON.stringify({ event: "pong" }),
        interval: 60000,
        pongTimeout: 5000
    },
    onConnected: (ws) => {
        const token = getToken();
        if (token) ws.send(JSON.stringify({ event: "adminAuth", data: token }));
    },
    onDisconnected: () => {
        webStore.interruptQueries();
    },
    onError: (ws, event) => {
        console.error("Got error on websocket eventPhase: ", event.eventPhase);
    },
    onMessage: (ws, msg) => {
        let msgObj;
        try {
            msgObj = JSON.parse(msg.data);
        } catch {
            return;
        }
        // The batch page reads durable tasks over HTTP; the legacy per-second bulkCheckInfo push is ignored.
        switch (msgObj.event) {
            case "webCheckDomains":
                webStore.setCheckingDomains(msgObj.data, "webCheck", "");
                break;
            case "webCheckStopped":
                webStore.stopQueries();
                Notify.create({ position: "top", type: "info", message: "已停止查询，已完成的结果已保留" });
                break;
            case "typoResult":
                webStore.setCheckingDomains(msgObj.data.domains, "typoCheck", msgObj.data.typoType);
                break;
            case "webCheckResult":
                webStore.updateCheckResult(msgObj.data);
                break;
            case "registerResult":
                webStore.updateRegisterResult(msgObj.data);
                break;
            case "registerError":
                webStore.finishPendingRegistrations("error", msgObj.data);
                Notify.create({ type: "negative", message: msgObj.data });
                break;
            case "bulkCheckError":
            case "webCheckError":
            case "typoCheckError":
                if (msgObj.event !== "bulkCheckError") {
                    webStore.pendingQuery = null;
                    webStore.pendingScope = null;
                }
                Notify.create({
                    position: "top",
                    type: "negative",
                    message: "错误: " + msgObj.data,
                    progress: true,
                    timeout: 10000,
                    actions: [{ label: "确定", color: "white" }]
                });
                break;
        }
    }
});

// retryScope names the tab ("webCheck" or "typoCheck") whose rows a retry replaces.
export function submitQuery(event, data, retryScope = null) {
    if (webStore.busy) return;
    if (send(JSON.stringify({ event, data }), false)) {
        webStore.pendingQuery = retryScope ? "retry" : event;
        webStore.pendingScope = retryScope || event;
        webStore.pendingMode = data.queryType || null;
    } else {
        Notify.create({ position: "top", type: "warning", message: "连接已断开，请重连后再提交查询。" });
    }
}

export function stopQuery() {
    if (webStore.pendingQuery || !webStore.unCheckDomains) return;
    if (send(JSON.stringify({ event: "webCheckStop", data: null }), false)) {
        webStore.pendingQuery = "stop";
    }
}
