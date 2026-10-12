import { defineStore } from 'pinia'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('hermeshub_token') || '',
  }),
  actions: {
    setToken(token) {
      this.token = token
      localStorage.setItem('hermeshub_token', token)
    },
    logout() {
      this.token = ''
      localStorage.removeItem('hermeshub_token')
    },
  },
})
