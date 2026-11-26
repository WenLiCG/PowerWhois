export function formatQueryError(message) {
    if (!message || typeof message !== "string") return "";
    const trimmed = message.trim();
    const lower = trimmed.toLowerCase();
    if (lower.includes("context deadline exceeded") || lower.includes("whois query timeout") || lower.includes("dns query timeout") || lower.includes("i/o timeout")) {
        if (lower.includes("connect to proxy failed")) {
            return "SOCKS5 代理连接超时，请检查代理主机、端口与网络连通性";
        }
        return "查询超时（超过单域名时限，可点击重查或增大超时设置）";
    }
    if (lower.includes("connect to proxy failed")) {
        const idx = trimmed.indexOf("（");
        if (idx >= 0) return "代理连接失败" + trimmed.slice(idx);
        return "SOCKS5 代理连接失败，请到「系统设置」测试代理连通性";
    }
    if (lower.includes("dns has no delegation")) {
        return "DNS 无委派记录（不代表未注册，请改用 WHOIS/RDAP 核实）";
    }
    if (lower.includes("registry rate limited")) {
        return "注册局触发限流，已自动退避，可稍后重查";
    }
    if (lower.includes("ac.uk requires a jisc")) {
        return "暂不支持 .ac.uk 后缀的注册库查询（可使用 DNS 查看委派）";
    }
    if (lower.includes("not supported tld")) {
        return "暂不支持该后缀的 WHOIS / RDAP 查询";
    }
    if (lower.includes("domain reserved by registry")) {
        return "注册局保留域名";
    }
    if (lower.includes("no parsing rule for tld") || lower.includes("parse whois response failed")) {
        return "无法解析注册局响应，请点击查看原始响应";
    }
    if (lower.includes("no content in whois response")) {
        return "WHOIS 服务器返回空响应";
    }
    if (lower.includes("whois server failed")) {
        return "上游 WHOIS / RDAP 服务器请求失败，请稍后重查或切换代理";
    }
    if (lower.includes("dns server failed")) {
        return "权威 DNS 查询失败";
    }
    if (lower.includes("customize api server response error")) {
        return "自定义查询接口返回异常";
    }
    if (lower.includes("customize api whois result error")) {
        return "自定义查询接口响应未命中配置的匹配词";
    }
    return trimmed;
}

export function formatDateTime(value) {
    if (!value) return "—";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return String(value);
    const pad = (n) => String(n).padStart(2, "0");
    return `${parsed.getFullYear()}-${pad(parsed.getMonth() + 1)}-${pad(parsed.getDate())} ${pad(parsed.getHours())}:${pad(parsed.getMinutes())}:${pad(parsed.getSeconds())}`;
}
