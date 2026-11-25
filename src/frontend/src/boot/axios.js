
import axios from "axios";
import { getToken, removeToken } from "src/utils/tokenHandler";

// Be careful when using SSR for cross-request state pollution
// due to creating a Singleton instance here;
// If any client changes this (global) instance, it might be a
// good idea to move this instance creation inside of the
// "export default () => {}" function below (which runs individually
// for each client)

var apiUrl = "/api";


const api = axios.create({
    baseURL: apiUrl,
    timeout: 5000,
    headers: {
        "Content-Type": "application/json"
    }
});

export default ({ app }) => {
    // for use inside Vue files (Options API) through this.$axios and this.$api

    app.config.globalProperties.$axios = axios;
    // ^ ^ ^ this will allow you to use this.$axios (for Vue Options API form)
    //       so you won't necessarily have to import axios in each vue file

    app.config.globalProperties.$api = api;
    // ^ ^ ^ this will allow you to use this.$api (for Vue Options API form)
    //       so you can easily perform requests against your app's API

    // 通过拦截器处理token
    api.interceptors.request.use((config) => {
        const token = getToken();
        if (token) {
            config.headers.Authorization = `Bearer ${token}`;
        }
        return config;
    });

    // 通过拦截器处理跳转
    api.interceptors.response.use(
        (response) => {
            return response;
        },
        (error) => {
            if (error.response?.status === 401 && error.config?.headers?.Authorization === `Bearer ${getToken()}`) {
                // 未认证，或认证失败，删除token
                removeToken();
            }
            return Promise.reject(error);
        }
    );
};

export { api, apiUrl, axios };
