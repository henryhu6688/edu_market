import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'

const routes = [
  {
    path: '/',
    name: 'Home',
    component: () => import('@/views/Home.vue'),
    meta: { title: '首页 - EduMarket' }
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { title: '登录 - EduMarket', guest: true }
  },
  {
    path: '/course/:id',
    name: 'CourseDetail',
    component: () => import('@/views/CourseDetail.vue'),
    meta: { title: '课程详情 - EduMarket' }
  },
  {
    path: '/profile',
    name: 'Profile',
    component: () => import('@/views/Profile.vue'),
    meta: { title: '个人中心 - EduMarket', auth: true }
  },
  {
    path: '/orders',
    name: 'Orders',
    component: () => import('@/views/Orders.vue'),
    meta: { title: '我的订单 - EduMarket', auth: true }
  },
  {
    path: '/materials/new',
    name: 'PublishMaterial',
    component: () => import('@/views/MaterialEditor.vue'),
    meta: { title: '发布资料 - EduMarket', auth: true }
  },
  {
    path: '/materials',
    name: 'Materials',
    component: () => import('@/views/MaterialList.vue'),
    meta: { title: '学习资料 - EduMarket' }
  },
  {
    path: '/materials/:id',
    name: 'MaterialDetail',
    component: () => import('@/views/MaterialDetail.vue'),
    meta: { title: '资料详情 - EduMarket' }
  },
  {
    path: '/materials/:mid/docs',
    name: 'DocEditor',
    component: () => import('@/views/DocumentEditor.vue'),
    meta: { title: '文档编辑器 - EduMarket', auth: true }
  },
  {
    path: '/materials/:mid/docs/:did',
    name: 'DocView',
    component: () => import('@/views/DocumentView.vue'),
    meta: { title: '文档阅读 - EduMarket', auth: true }
  },
  {
    path: '/agent',
    name: 'AgentChat',
    component: () => import('@/views/AgentChat.vue'),
    meta: { title: 'AI 助手 - EduMarket', auth: true }
  },
  {
    path: '/admin',
    redirect: '/admin/dashboard',
    meta: { auth: true, admin: true }
  },
  {
    path: '/admin/dashboard',
    name: 'AdminDashboard',
    component: () => import('@/views/admin/Dashboard.vue'),
    meta: { title: '管理后台 - EduMarket', auth: true, admin: true }
  },
  {
    path: '/admin/courses',
    name: 'AdminCourses',
    component: () => import('@/views/admin/CourseManage.vue'),
    meta: { title: '课程管理 - EduMarket', auth: true, admin: true }
  },
  {
    path: '/admin/categories',
    name: 'AdminCategories',
    component: () => import('@/views/admin/CategoryManage.vue'),
    meta: { title: '分类管理 - EduMarket', auth: true, admin: true }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// 路由守卫
router.beforeEach(async (to, from, next) => {
  document.title = to.meta.title || 'EduMarket'
  const userStore = useUserStore()

  // 首次访问且本地有 token — 先验证有效性
  if (!userStore.isVerified && userStore.accessToken) {
    const valid = await userStore.verifyAuth()
    if (!valid && to.meta.auth) {
      return next('/login')
    }
  }

  if (to.meta.auth && !userStore.isLoggedIn) {
    return next('/login')
  }
  if (to.meta.admin && userStore.user?.role !== 'admin') {
    return next('/')
  }
  if (to.meta.guest && userStore.isLoggedIn) {
    return next('/')
  }
  next()
})

export default router
