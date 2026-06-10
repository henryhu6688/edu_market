import api from './index'

// SSE 对话（返回 fetch response，由调用方读取 ReadableStream）
export function agentChat({ session_id, question }, token) {
  return fetch('/api/agent/chat', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`
    },
    body: JSON.stringify({ session_id, question })
  })
}

// 会话列表
export function getSessions(params) {
  return api.get('/agent/sessions', { params })
}

// 删除会话
export function deleteSession(id) {
  return api.delete(`/agent/sessions/${id}`)
}

// 获取会话历史消息
export function getMessages(id, params) {
  return api.get(`/agent/sessions/${id}/messages`, { params })
}
