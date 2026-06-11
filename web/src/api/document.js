import api from './index'

export function getDocTree(materialId) { return api.get(`/materials/${materialId}/documents`) }
export function getDocument(id) { return api.get(`/documents/${id}`) }
export function createDocument(materialId, data) { return api.post(`/materials/${materialId}/documents`, data) }
export function updateDocument(id, data) { return api.put(`/documents/${id}`, data) }
export function deleteDocument(id) { return api.delete(`/documents/${id}`) }
export function uploadFile(materialId, formData) {
  return api.post(`/materials/${materialId}/documents/upload`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}
