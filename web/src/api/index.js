import axios from 'axios'
import { useUserStore } from '@/stores/user'
import router from '@/router'
import { refreshToken } from './auth'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000
})

// 请求拦截器 — 自动带 Access Token
api.interceptors.request.use(config => {
  const userStore = useUserStore()
  if (userStore.accessToken) {
    config.headers.Authorization = `Bearer ${userStore.accessToken}`
  }
  return config
})

// 响应拦截器 — 401 自动刷新 Token
let isRefreshing = false
let refreshQueue = []

api.interceptors.response.use(
  response => response.data,
  async error => {
    const { config, response } = error

    if (response?.status === 401 && !config._retry) {
      const userStore = useUserStore()

      if (userStore.refreshToken) {
        config._retry = true

        if (isRefreshing) {
          // 正在刷新中，排队等待
          return new Promise((resolve, reject) => {
            refreshQueue.push({ resolve, reject })
          }).then(() => {
            config.headers.Authorization = `Bearer ${userStore.accessToken}`
            return api(config)
          })
        }

        isRefreshing = true
        try {
          const data = await refreshToken(userStore.refreshToken)
          userStore.updateAccessToken(data.data.access_token, data.data.refresh_token)
          refreshQueue.forEach(q => q.resolve())
          refreshQueue = []
          config.headers.Authorization = `Bearer ${userStore.accessToken}`
          return api(config)
        } catch (e) {
          refreshQueue.forEach(q => q.reject(e))
          refreshQueue = []
          userStore.logout()
          router.push('/login')
          return Promise.reject(e)
        } finally {
          isRefreshing = false
        }
      } else {
        userStore.logout()
        router.push('/login')
      }
    }

    return Promise.reject(response?.data || error)
  }
)

export default api
