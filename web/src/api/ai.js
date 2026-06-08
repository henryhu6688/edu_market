import api from './index'

export function chat(data) {
  return api.post('/ai/chat', data)
}

export function getHistory(params) {
  return api.get('/ai/history', { params })
}
