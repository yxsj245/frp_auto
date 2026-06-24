import { createRouter, createWebHashHistory } from 'vue-router'
import ApplyView from '../views/ApplyView.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/apply' },
    { path: '/apply', name: 'Apply', component: ApplyView },
  ],
})

export default router
