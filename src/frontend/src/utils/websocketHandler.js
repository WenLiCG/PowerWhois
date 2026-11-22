import { ref } from "vue";
import { useWebSocket } from "@vueuse/core";
import { api } from "boot/axios";
import { getToken, removeToken } from "./tokenHandler";
import { useBulkStore } from "src/stores/bulkStore";
import { useWebStore } from "src/stores/webStore";
import { Notify } from "quasar";

const wsBase = process.env.DEV ? process.env.QENV_API_URL + "/app/ws" : "/app/ws";
const bulkStore = useBulkStore();
const webStore = useWebStore();

// Signed-in administrators connect with a one-time ticket, which admits them
// into connections reserved for administrators even when anonymous visitors
// fill the public ones. Each connection needs a new ticket, so this module
// reconnects itself instead of using VueUse autoReconnect (which would reuse
// the spent ticket in the URL).
const wsTarget = ref(undefined);
let stopped = false;
let connecting = false;
let reconnectTimer = null;

async function nextTarget() {
    const token = getToken();
    if (!token) return wsBase;
    try {
        const { data } = await api.post("/admin/ws-ticket", null, { headers: { Authorization: `Bearer ${token}` } });
        if (data?.ticket) return `${wsBase}?ticket=${encodeURIComponent(data.ticket)}`;
    } catch {
        // Fall back to an ordinary connection; adminAuth below still signs it in.
    }
    return wsBase;
}

async function connect() {
    if (connecting || stopped) return;
    connecting = true;
    try {
        const target = await nextTarget();
        if (stopped) return;
        // A changed URL makes useWebSocket open the new connection itself.
        if (wsTarget.value === target) socket.open();
        else wsTarget.value = target;
    } finally {
        connecting = false;
    }
}

function scheduleReconnect() {
    if (stopped || reconnectTimer) return;
    reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connect();
    }, 2000);
}

const socket = useWebSocket(wsTarget, {
    immediate: false,
    autoReconnect: false,
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
    onDisconnected: (ws) => {
        webStore.finishCheck();
        webStore.finishRegistration();
        // Ignore sockets replaced by a newer connection.
        if (ws === socket.ws.value) scheduleReconnect();
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

export const { status } = socket;

// close stops the connection for good, as on logout or page teardown.
export function close() {
    stopped = true;
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
    socket.close();
}

connect();

export function send(data) {
    if (socket.status.value === "OPEN" && socket.send(data, false)) return true;
    notifyError("连接中断，请重连后再操作");
    return false;
}

function notifyError(message) {
    Notify.create({ position: "top", type: "negative", message, timeout: 10000 });
}
