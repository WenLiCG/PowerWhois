import { createRouter, createWebHistory } from 'vue-router';
import routes from './routes';
export default () => createRouter({ history: createWebHistory(), routes, scrollBehavior: () => ({ left: 0, top: 0 }) });
