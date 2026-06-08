import api from './index'

export function getCourses(params) {
  return api.get('/courses', { params })
}

export function getCourseDetail(id) {
  return api.get(`/courses/${id}`)
}

export function createCourse(data) {
  return api.post('/admin/courses', data)
}

export function updateCourse(id, data) {
  return api.put(`/admin/courses/${id}`, data)
}

export function deleteCourse(id) {
  return api.delete(`/admin/courses/${id}`)
}
