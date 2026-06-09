import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useUserStore = defineStore('user', () => {
  const accessToken = ref(localStorage.getItem('access_token') || '')
  const refreshToken = ref(localStorage.getItem('refresh_token') || '')
  const user = ref(JSON.parse(localStorage.getItem('user') || 'null'))

  const isLoggedIn = computed(() => !!accessToken.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  function setAuth(access, refresh, newUser) {
    accessToken.value = access
    refreshToken.value = refresh
    user.value = newUser
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
    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
    localStorage.removeItem('user')
  }

  function updateUser(updates) {
    user.value = { ...user.value, ...updates }
    localStorage.setItem('user', JSON.stringify(user.value))
  }

  return { accessToken, refreshToken, user, isLoggedIn, isAdmin, setAuth, updateAccessToken, logout, updateUser }
})
