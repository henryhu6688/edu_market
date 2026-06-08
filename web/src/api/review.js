import api from './index'

export function createReview(data) {
  return api.post('/reviews', data)
}

export function getReviews(courseId, params) {
  return api.get(`/courses/${courseId}/reviews`, { params })
}
