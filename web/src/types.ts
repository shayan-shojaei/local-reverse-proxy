export type PublicMode = 'http' | 'https'
export type Health = 'unknown' | 'reachable' | 'unreachable'

export interface Upstream {
  scheme: 'http' | 'https'
  host: string
  port: number
  skipTlsVerify: boolean
}

export interface Route {
  id: string
  hostname: string
  enabled: boolean
  publicMode: PublicMode
  upstream: Upstream
  revision: number
  health: Health
  createdAt: string
  updatedAt: string
}

export interface RouteDraft {
  hostname: string
  publicMode: PublicMode
  upstream: Upstream
}

export interface SystemStatus {
  controller: 'healthy' | 'degraded'
  zone: string
}
