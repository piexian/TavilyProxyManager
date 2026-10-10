import axios from 'axios'
import { ref } from 'vue'

const STORAGE_KEY = 'tavily_proxy_master_key'
const ROLE_KEY = 'tavily_proxy_role'

// 两种角色共用同一个 Bearer token（master key 或捐赠者访问密钥）；role 用于刷新后恢复正确界面。
export type AuthRole = '' | 'admin' | 'donor'

const masterKeyRef = ref<string>(localStorage.getItem(STORAGE_KEY) ?? '')
const roleRef = ref<AuthRole>((localStorage.getItem(ROLE_KEY) as AuthRole) || '')

export function getMasterKey(): string {
  return masterKeyRef.value
}

export function setMasterKey(value: string): void {
  localStorage.setItem(STORAGE_KEY, value)
  masterKeyRef.value = value
}

export function clearMasterKey(): void {
  localStorage.removeItem(STORAGE_KEY)
  masterKeyRef.value = ''
  clearRole()
}

export function getRole(): AuthRole {
  return roleRef.value
}

export function setRole(role: AuthRole): void {
  localStorage.setItem(ROLE_KEY, role)
  roleRef.value = role
}

export function clearRole(): void {
  localStorage.removeItem(ROLE_KEY)
  roleRef.value = ''
}

export const api = axios.create()

api.interceptors.request.use((config) => {
  const token = getMasterKey()
  if (token) {
    config.headers = config.headers ?? {}
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error?.response?.status === 401) {
      clearMasterKey()
      window.dispatchEvent(new Event('auth-required'))
    }
    return Promise.reject(error)
  }
)
