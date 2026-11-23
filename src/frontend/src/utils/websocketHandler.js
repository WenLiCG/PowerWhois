import { useWebSocket } from "@vueuse/core";
import { getToken } from "./tokenHandler";
import { useBulkStore } from "src/stores/bulkStore";
import { useWebStore } from "src/stores/webStore";
import { Notify } from "quasar";

const wsUrl = new URL('/app/ws', window.location.href);
wsUrl.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';

const bulkStore = useBulkStore();
const webStore = useWebStore();

export const { status, send, open, close } = useWebSocket(wsUrl.href, {
    autoReconnect: true,
    heartbeat: {
        message: JSON.stringify({ event: "ping" }),
        responseMessage: JSON.stringify({ event: "pong" }),
        interval: 60000, //60秒
        pongTimeout: 5000 //5秒
    },
    onConnected: (ws) => {
        const token = getToken();
        if (token) ws.send(JSON.stringify({ event: "adminAuth", data: token }));
    },
    onDisconnected: (ws, event) => {
        webStore.interruptQueries();
    },
    onError: (ws, event) => {
        console.error("Got error on websocket eventPhase: ", event.eventPhase);
    },
    onMessage: (ws, msg) => {
        let msgObj;
        try { msgObj = JSON.parse(msg.data); } catch { return; }
        switch (msgObj.event) {
            case "bulkCheckInfo":
                bulkStore.setBulkCheckStatus(msgObj.data.Status);
                bulkStore.setBulkCheckQueryType(msgObj.data.QueryType);
                bulkStore.updateBulkCheckInfo(msgObj.data);
                break;
            case "webCheckDomains":
                webStore.setCheckingDomains(msgObj.data, "webCheck", "");
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
            case "bulkCheckError":
            case "webCheckError":
            case "typoCheckError":
            case "registerError":
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
