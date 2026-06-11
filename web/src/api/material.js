import api from './index'

export function getMaterials(params) { return api.get('/materials', { params }) }
export function getMaterial(id) { return api.get(`/materials/${id}`) }
export function createMaterial(data) { return api.post('/materials', data) }
export function updateMaterial(id, data) { return api.put(`/materials/${id}`, data) }
export function deleteMaterial(id) { return api.delete(`/materials/${id}`) }
export function getMyMaterials(params) { return api.get('/user/materials', { params }) }
