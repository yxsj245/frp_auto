import { createRouter, createWebHashHistory } from 'vue-router'
import AssistanceList from '../views/AssistanceList.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/assistance' },
    { path: '/assistance', name: 'Assistance', component: AssistanceList },
  ],
})

export default router
