import { createRouter, createWebHistory } from 'vue-router'
import LoginView from '../views/Login.vue'
import InboxView from '../views/Inbox.vue'
import CaseView from '../views/CaseView.vue'
import PasswordChangeView from '../views/PasswordChange.vue'
import { installAuthGuards } from './guards'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/inbox' },
    { path: '/login', component: LoginView },
    { path: '/inbox', component: InboxView, meta: { requiresAuth: true } },
    { path: '/cases/:id', component: CaseView, meta: { requiresAuth: true } },
    { path: '/auth/password', component: PasswordChangeView, meta: { requiresAuth: true } },
  ],
})

installAuthGuards(router)

export default router
