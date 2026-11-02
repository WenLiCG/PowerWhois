export function actionsFor(job) {
    if (!job) return [];
    switch (job.state) {
        case "draft":
            return ["cancel"];
        case "ready":
            return job.candidates ? ["start", "cancel"] : ["cancel"];
        case "running":
            return ["pause", "cancel"];
        case "paused":
            return ["resume", "cancel"];
        case "error":
            return ["cancel"];
        default:
            return [];
    }
}
export function columnsFor(kind) {
    if (kind === "seeds") return ["sheet", "row", "raw", "domain", "duplicate", "error"];
    if (kind === "origins") return ["candidate", "domain", "sheet", "row", "raw", "rule"];
    return ["domain", "status", "checkedAt", "httpStatus", "error", "evidence"];
}
export function savedOptions(job) {
    return job.options?.rules?.length
        ? job.options
        : { rules: ["omission", "repetition", "transposition"], maxCandidates: 500000, tlds: ["com", "net", "org", "co"] };
}
export function stateLabel(state) {
    return (
        {
            draft: "待生成候选",
            importing: "正在导入",
            generating: "正在生成候选",
            ready: "等待开始查询",
            running: "正在查询",
            paused: "已暂停",
            completed: "查询完成",
            canceled: "已取消",
            registered: "已注册",
            not_found: "未找到",
            unknown: "无法确定",
            error: "错误",
            pending: "待查询"
        }[state] || state
    );
}
export const ruleOptions = [
    { label: "删除字符", value: "omission" },
    { label: "重复字符", value: "repetition" },
    { label: "相邻换位", value: "transposition" },
    { label: "插入字符", value: "insertion" },
    { label: "键盘横向替换", value: "horizontal" },
    { label: "键盘纵向替换", value: "vertical" },
    { label: "添加 www 前缀", value: "www" },
    { label: "替换后缀", value: "tld" }
];
