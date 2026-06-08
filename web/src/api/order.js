import api from './index'

export function createOrder(data) {
  return api.post('/orders', data)
}

export function getOrders(params) {
  return api.get('/orders', { params })
}

export function payOrder(orderNo) {
  return api.post(`/orders/${orderNo}/pay`)
}
