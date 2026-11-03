export const rules = {
    required: (message) => (value) => (value !== null && value !== undefined && String(value).trim().length > 0) || message,
    numeric: (message) => (value) => (value !== "" && value !== null && Number.isFinite(Number(value))) || message,
    minValue: (min, message) => (value) => Number(value) >= min || message,
    maxValue: (max, message) => (value) => Number(value) <= max || message
};
export default ({ app }) => {
    app.config.globalProperties.$rules = rules;
};
