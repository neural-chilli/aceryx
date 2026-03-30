import { createRouter, createWebHistory } from 'vue-router'
import LoginView from '../views/Login.vue'
import InboxView from '../views/Inbox.vue'
import CaseListView from '../views/CaseList.vue'
import CaseView from '../views/CaseView.vue'
import PasswordChangeView from '../views/PasswordChange.vue'
import BuilderView from '../views/Builder.vue'
import ReportsView from '../views/Reports.vue'
import ActivityView from '../views/Activity.vue'
import { installAuthGuards } from './guards'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/inbox' },
    { path: '/login', component: LoginView },
    { path: '/inbox', component: InboxView, meta: { requiresAuth: true } },
    { path: '/activity', component: ActivityView, meta: { requiresAuth: true } },
    { path: '/cases', component: CaseListView, meta: { requiresAuth: true } },
    { path: '/cases/:id', component: CaseView, meta: { requiresAuth: true } },
    { path: '/builder', component: BuilderView, meta: { requiresAuth: true } },
    { path: '/reports', component: ReportsView, meta: { requiresAuth: true } },
    { path: '/auth/password', component: PasswordChangeView, meta: { requiresAuth: true } },
  ],
})

installAuthGuards(router)

export default router
