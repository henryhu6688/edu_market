import api from './index'

// 获取图形验证码
export function getCaptcha() {
  return api.get('/captcha/image')
}

// 发送短信验证码（需传图形验证码）
export function sendCode(data) {
  return api.post('/send-code', data)
}

// 统一登录/注册（手机号+验证码）
export function loginByCode(data) {
  return api.post('/login', data)
}

// 获取当前用户信息（验证 token 是否有效）
export function getProfile() {
  return api.get('/user/profile')
}

// 刷新 Token
export function refreshToken(refreshToken) {
  return api.post('/refresh', { refresh_token: refreshToken })
}
