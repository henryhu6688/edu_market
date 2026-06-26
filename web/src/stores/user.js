import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getProfile } from '@/api/auth'

export const useUserStore = defineStore('user', () => {
  const accessToken = ref(localStorage.getItem('access_token') || '')
  const refreshToken = ref(localStorage.getItem('refresh_token') || '')
  const user = ref(JSON.parse(localStorage.getItem('user') || 'null'))
  const isVerified = ref(false) // token 是否已通过后端校验

  const isLoggedIn = computed(() => !!accessToken.value && isVerified.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  function setAuth(access, refresh, newUser) {
    accessToken.value = access
    refreshToken.value = refresh
    user.value = newUser
    isVerified.value = true
    localStorage.setItem('access_token', access)
    localStorage.setItem('refresh_token', refresh)
    localStorage.setItem('user', JSON.stringify(newUser))
  }

  function updateAccessToken(access, refresh) {
    accessToken.value = access
    refreshToken.value = refresh
    localStorage.setItem('access_token', access)
    if (refresh) localStorage.setItem('refresh_token', refresh)
  }

  function logout() {
    accessToken.value = ''
    refreshToken.value = ''
    user.value = null
    isVerified.value = false
    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
    localStorage.removeItem('user')
  }

  function updateUser(updates) {
    user.value = { ...user.value, ...updates }
    localStorage.setItem('user', JSON.stringify(user.value))
  }

  // verifyAuth 向后端验证 token 是否仍然有效
  async function verifyAuth() {
    if (!accessToken.value) {
      isVerified.value = false
      return false
    }
    try {
      const data = await getProfile()
      user.value = data.data
      isVerified.value = true
      return true
    } catch {
      logout()
      return false
    }
  }

  return { accessToken, refreshToken, user, isLoggedIn, isAdmin, isVerified,
    setAuth, updateAccessToken, logout, updateUser, verifyAuth }
})
