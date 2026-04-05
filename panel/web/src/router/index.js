import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', component: () => import('../views/Login.vue') },
  {
    path: '/',
    component: () => import('../views/Layout.vue'),
    redirect: '/dashboard',
    children: [
      { path: 'dashboard', component: () => import('../views/Dashboard.vue') },
      { path: 'upstreams', component: () => import('../views/Upstreams.vue') },
      { path: 'servers', component: () => import('../views/Servers.vue') },
      { path: 'routes', component: () => import('../views/Routes.vue') },
      { path: 'rules', component: () => import('../views/Rules.vue') },
    ],
  },
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach((to) => {
  if (to.path !== '/login' && !localStorage.getItem('token')) return '/login'
})

export default router
