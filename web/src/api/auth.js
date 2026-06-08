import api from './index'

// 用户名密码登录
export function login(data) {
  return api.post('/login', data)
}

// 用户名注册
export function register(data) {
  return api.post('/register', data)
}

// 发送手机验证码
export function sendCode(phone) {
  return api.post('/send-code', { phone })
}

// 手机号验证码注册
export function registerByPhone(data) {
  return api.post('/register/phone', data)
}

// 手机号验证码登录
export function loginByPhone(data) {
  return api.post('/login/phone', data)
}
