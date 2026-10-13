import axios from 'axios'
import { message } from 'ant-design-vue'
import { useAuthStore } from '../stores/auth'
import router from '../router'

const api = axios.create({ baseURL: '/api', timeout: 30000 })

api.interceptors.request.use((config) => {
  const auth = useAuthStore()
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  return config
})

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      const auth = useAuthStore()
      auth.logout()
      router.push({ name: 'login' })
    } else {
      const msg = err.response?.data?.error || err.message || '请求失败'
      message.error(msg)
    }
    return Promise.reject(err)
  }
)

export default api

// ---------- 实例 ----------
export const listInstances = () => api.get('/instances')
export const createInstance = (data) => api.post('/instances', data)
export const updateInstance = (id, data) => api.patch(`/instances/${id}`, data)
export const deleteInstance = (id) => api.delete(`/instances/${id}`)

// ---------- 项目 ----------
export const listProjects = () => api.get('/projects')
export const getProject = (id) => api.get(`/projects/${id}`)
export const createProject = (data) => api.post('/projects', data)
export const updateProject = (id, data) => api.patch(`/projects/${id}`, data)
export const deleteProject = (id) => api.delete(`/projects/${id}`)

// ---------- 成员 ----------
export const listMembers = (pid) => api.get(`/projects/${pid}/members`)
export const addMember = (pid, data) => api.post(`/projects/${pid}/members`, data)
export const updateMember = (pid, mid, data) => api.patch(`/projects/${pid}/members/${mid}`, data)
export const removeMember = (pid, mid) => api.delete(`/projects/${pid}/members/${mid}`)

// ---------- 消息 ----------
export const listMessages = (pid, params) => api.get(`/projects/${pid}/messages`, { params })
export const sendMessage = (pid, content, replyTo) =>
  api.post(`/projects/${pid}/messages`, { content, ...(replyTo != null ? { reply_to: replyTo } : {}) })

// ---------- 看板 ----------
export const listCards = (pid) => api.get(`/projects/${pid}/cards`)
export const createCard = (pid, data) => api.post(`/projects/${pid}/cards`, data)
export const updateCard = (pid, cid, data) => api.patch(`/projects/${pid}/cards/${cid}`, data)
export const deleteCard = (pid, cid) => api.delete(`/projects/${pid}/cards/${cid}`)

// ---------- 状态总结 ----------
export const getSummary = (pid) => api.get(`/projects/${pid}/summary`)
export const putSummary = (pid, content) => api.put(`/projects/${pid}/summary`, { content })

// ---------- 派发 ----------
export const listDispatches = (pid, params) => api.get(`/projects/${pid}/dispatches`, { params })

// ---------- 文件 ----------
export const listFiles = (pid) => api.get(`/projects/${pid}/files`)
export const deleteFile = (pid, path) => api.delete(`/projects/${pid}/files/${encodeURI(path)}`)
export const uploadFile = (pid, file, dir = '') => {
  const fd = new FormData()
  fd.append('file', file)
  fd.append('dir', dir)
  return api.post(`/projects/${pid}/files/upload`, fd)
}
export const downloadFile = async (pid, path) => {
  const res = await api.get(`/projects/${pid}/files/${encodeURI(path)}`, { responseType: 'blob' })
  const url = URL.createObjectURL(res.data)
  const a = document.createElement('a')
  a.href = url
  a.download = path.split('/').pop()
  a.click()
  URL.revokeObjectURL(url)
}
