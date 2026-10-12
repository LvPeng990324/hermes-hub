import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const routes = [
  { path: '/login', name: 'login', component: () => import('../views/Login.vue'), meta: { public: true } },
  {
    path: '/',
    component: () => import('../layouts/MainLayout.vue'),
    children: [
      { path: '', redirect: '/projects' },
      { path: 'projects', name: 'projects', component: () => import('../views/Projects.vue') },
      { path: 'instances', name: 'instances', component: () => import('../views/Instances.vue') },
      { path: 'projects/:id', name: 'project-detail', component: () => import('../views/project/ProjectDetail.vue') },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.token) {
    return { name: 'login' }
  }
  if (to.name === 'login' && auth.token) {
    return { path: '/' }
  }
})

export default router
