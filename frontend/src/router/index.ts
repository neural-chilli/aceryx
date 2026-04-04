import { createRouter, createWebHistory } from 'vue-router'
import { installAuthGuards } from './guards'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', redirect: '/inbox' },
    { path: '/login', component: () => import('../views/Login.vue') },
    { path: '/inbox', component: () => import('../views/Inbox.vue'), meta: { requiresAuth: true } },
    { path: '/activity', component: () => import('../views/Activity.vue'), meta: { requiresAuth: true } },
    { path: '/cases', component: () => import('../views/CaseList.vue'), meta: { requiresAuth: true } },
    { path: '/cases/:id', component: () => import('../views/CaseView.vue'), meta: { requiresAuth: true } },
    { path: '/builder', component: () => import('../views/Builder.vue'), meta: { requiresAuth: true } },
    { path: '/reports', component: () => import('../views/Reports.vue'), meta: { requiresAuth: true } },
    { path: '/profile', component: () => import('../views/Profile.vue'), meta: { requiresAuth: true } },
    { path: '/auth/password', component: () => import('../views/PasswordChange.vue'), meta: { requiresAuth: true } },
  ],
})

installAuthGuards(router)

export default router
