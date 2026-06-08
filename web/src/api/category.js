import api from './index'

export function getCategories() {
  return api.get('/categories')
}

export function createCategory(data) {
  return api.post('/admin/categories', data)
}

export function updateCategory(id, data) {
  return api.put(`/admin/categories/${id}`, data)
}

export function deleteCategory(id) {
  return api.delete(`/admin/categories/${id}`)
}
