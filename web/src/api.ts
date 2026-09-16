import type { Route, RouteDraft, SystemStatus } from './types'

interface Envelope<T> { data: T }
interface Problem { detail?: string; title?: string }

export interface PortableConfig {
  version: 1
  zone: string
  routes: Array<RouteDraft & { enabled: boolean }>
}

export interface ImportPreview {
  digest: string
  targetZone: string
  added: number
  updated: number
  deleted: number
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json', ...init.headers } : init?.headers,
  })
  if (!response.ok) {
    const problem = await response.json().catch(() => ({})) as Problem
    throw new Error(problem.detail || problem.title || `Request failed (${response.status})`)
  }
  if (response.status === 204) return undefined as T
  const envelope = await response.json() as Envelope<T>
  return envelope.data
}

export const api = {
  exchangeToken: async (token: string) => {
    const response = await fetch('/api/v1/auth/exchange', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ token }),
    })
    if (!response.ok) throw new Error('This dashboard login link is invalid or expired.')
  },
  listRoutes: () => request<Route[]>('/routes'),
  status: () => request<SystemStatus>('/status'),
  createRoute: (draft: RouteDraft) => request<Route>('/routes', {
    method: 'POST', body: JSON.stringify(draft),
  }),
  updateRoute: (route: Route, draft: RouteDraft, enabled = route.enabled) => request<Route>(`/routes/${route.id}`, {
    method: 'PATCH', body: JSON.stringify({ ...draft, enabled, revision: route.revision }),
  }),
  deleteRoute: (route: Route) => request<void>(`/routes/${route.id}?revision=${route.revision}`, { method: 'DELETE' }),
  exportConfig: () => request<PortableConfig>('/config/export'),
  previewImport: (config: PortableConfig, mode: 'merge' | 'replace') => request<ImportPreview>('/config/import/preview', {
    method: 'POST', body: JSON.stringify({ config, mode }),
  }),
  applyImport: (digest: string) => request<void>('/config/import/apply', {
    method: 'POST', body: JSON.stringify({ digest }),
  }),
}
