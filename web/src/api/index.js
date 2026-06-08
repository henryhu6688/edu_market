import axios from 'axios'
import { useUserStore } from '@/stores/user'
import router from '@/router'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000
})

// 请求拦截器 — 自动带 Token
api.interceptors.request.use(config => {
  const userStore = useUserStore()
  if (userStore.token) {
    config.headers.Authorization = `Bearer ${userStore.token}`
  }
  return config
})

// 响应拦截器 — 统一错误处理
api.interceptors.response.use(
  response => response.data,
  error => {
    if (error.response?.status === 401) {
      const userStore = useUserStore()
      userStore.logout()
      router.push('/login')
    }
    return Promise.reject(error.response?.data || error)
  }
)

export default api
