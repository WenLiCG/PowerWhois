import { useWebSocket } from "@vueuse/core";
import { getToken, removeToken } from "./tokenHandler";
import { useBulkStore } from "src/stores/bulkStore";
import { useWebStore } from "src/stores/webStore";
import { Notify } from "quasar";

const wsUrl = process.env.DEV ? process.env.QENV_API_URL + "/app/ws" : "/app/ws";
const bulkStore = useBulkStore();
const webStore = useWebStore();

const socket = useWebSocket(wsUrl, {
    autoReconnect: { delay: 2000 },
    heartbeat: {
        message: JSON.stringify({ event: "ping" }),
        responseMessage: JSON.stringify({ event: "pong" }),
        interval: 20000,
        pongTimeout: 5000
    },
    onConnected: (ws) => {
        const token = getToken();
        if (token) ws.send(JSON.stringify({ event: "adminAuth", data: token }));
    },
    onDisconnected: () => {
        webStore.finishCheck();
        webStore.finishRegistration();
    },
    onMessage: (_ws, msg) => {
        let message;
        try {
            message = JSON.parse(msg.data);
        } catch {
            return;
        }
        const { event, data } = message;
        switch (event) {
            case "adminAuthResult":
                if (!data) removeToken();
                break;
            case "bulkCheckInfo":
                bulkStore.setBulkCheckStatus(data.Status);
                bulkStore.setBulkCheckQueryType(data.QueryType);
                bulkStore.updateBulkCheckInfo(data);
                break;
            case "webCheckDomains":
                webStore.setCheckingDomains(data, "webCheck", "");
                break;
            case "typoResult":
                webStore.setCheckingDomains(data.domains, "typoCheck", data.typoType);
                break;
            case "webCheckResult":
                webStore.updateCheckResult(data);
                break;
            case "webCheckDone":
                webStore.finishCheck();
                break;
            case "registerResult":
                webStore.updateRegisterResult(data);
                break;
            case "registerDone":
                webStore.finishRegistration();
                break;
            case "webCheckError":
            case "typoCheckError":
                webStore.finishCheck();
                notifyError(data);
                break;
            case "registerError":
                webStore.finishRegistration(true);
                notifyError(data);
                break;
            case "bulkCheckError":
                notifyError(data);
                break;
        }
    }
});

export const { status, open, close } = socket;

export function send(data) {
    if (socket.status.value === "OPEN" && socket.send(data, false)) return true;
    notifyError("连接中断，请重连后再操作");
    return false;
}

function notifyError(message) {
    Notify.create({ position: "top", type: "negative", message, timeout: 10000 });
}
