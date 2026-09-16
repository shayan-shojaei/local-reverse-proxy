import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, type PortableConfig } from './api'
import { RouteForm } from './components/RouteForm'
import { RouteTable } from './components/RouteTable'
import type { Route, RouteDraft, SystemStatus } from './types'

export default function App() {
  const [routes, setRoutes] = useState<Route[]>([])
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [pageError, setPageError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [editing, setEditing] = useState<Route | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [query, setQuery] = useState('')
  const importInput = useRef<HTMLInputElement>(null)

  const load = useCallback(async () => {
    try {
      const [nextRoutes, nextStatus] = await Promise.all([api.listRoutes(), api.status()])
      setRoutes(nextRoutes); setStatus(nextStatus); setPageError(null)
    } catch (error) { setPageError(error instanceof Error ? error.message : 'Could not load the dashboard') }
    finally { setLoading(false) }
  }, [])
  useEffect(() => {
    const initialize = async () => {
      const token = new URLSearchParams(window.location.hash.slice(1)).get('token')
      try {
        if (token) {
          window.history.replaceState(null, '', window.location.pathname)
          await api.exchangeToken(token)
        }
        await load()
      } catch (error) {
        setPageError(error instanceof Error ? error.message : 'Could not sign in')
        setLoading(false)
      }
    }
    void initialize()
  }, [load])

  const visible = useMemo(() => routes.filter((route) => `${route.hostname} ${route.upstream.host}`.includes(query.toLowerCase())), [routes, query])
  const openCreate = () => { setEditing(null); setFormError(null); setFormOpen(true) }
  const openEdit = (route: Route) => { setEditing(route); setFormError(null); setFormOpen(true) }
  const save = async (draft: RouteDraft) => {
    setSaving(true); setFormError(null)
    try {
      if (editing) await api.updateRoute(editing, draft)
      else await api.createRoute(draft)
      setFormOpen(false); await load()
    } catch (error) { setFormError(error instanceof Error ? error.message : 'Could not apply this route') }
    finally { setSaving(false) }
  }
  const remove = async (route: Route) => {
    if (!window.confirm(`Delete ${route.hostname}.${status?.zone ?? 'local.test'}?`)) return
    try { await api.deleteRoute(route); await load() }
    catch (error) { setPageError(error instanceof Error ? error.message : 'Could not delete route') }
  }
  const toggle = async (route: Route) => {
    const draft: RouteDraft = { hostname: route.hostname, publicMode: route.publicMode, upstream: route.upstream }
    try { await api.updateRoute(route, draft, !route.enabled); await load() }
    catch (error) { setPageError(error instanceof Error ? error.message : 'Could not update route') }
  }
  const exportRoutes = async () => {
    try {
      const config = await api.exportConfig()
      const url = URL.createObjectURL(new Blob([`${JSON.stringify(config, null, 2)}\n`], { type: 'application/json' }))
      const link = document.createElement('a'); link.href = url; link.download = `lrp-${config.zone}.json`; link.click(); URL.revokeObjectURL(url)
    } catch (error) { setPageError(error instanceof Error ? error.message : 'Could not export routes') }
  }
  const importRoutes = async (file: File) => {
    try {
      const config = JSON.parse(await file.text()) as PortableConfig
      const preview = await api.previewImport(config, 'merge')
      if (!window.confirm(`Import into ${preview.targetZone}? Add ${preview.added}, update ${preview.updated}, delete ${preview.deleted}.`)) return
      await api.applyImport(preview.digest); await load()
    } catch (error) { setPageError(error instanceof Error ? error.message : 'Could not import routes') }
    finally { if (importInput.current) importInput.current.value = '' }
  }

  const zone = status?.zone ?? 'local.test'
  return (
    <div className="app-shell">
      <header className="topbar"><a className="brand" href="/" aria-label="Local Reverse Proxy home"><span className="brand__mark">LRP</span><span>Local Reverse Proxy</span></a><span className="local-badge">Local only</span></header>
      <main>
        <section className="page-heading"><div><p className="eyebrow">Domain routing</p><h1>Your local apps, properly addressed.</h1><p>Give every development server a memorable <code>.test</code> URL with trusted HTTPS.</p></div><button className="button button--primary button--large" onClick={openCreate}>＋ Add route</button></section>
        <section className="system-strip" aria-label="System status"><div><span className={`health-dot ${status ? '' : 'health-dot--muted'}`} /><span><strong>{status ? 'Proxy services online' : 'Checking services…'}</strong><small>{zone} · loopback only</small></span></div><button className="button button--quiet" onClick={() => void load()}>Run checks</button></section>
        {pageError && <div className="page-error" role="alert"><span>{pageError}</span><button onClick={() => void load()}>Retry</button></div>}
        <section className="routes-section" aria-labelledby="routes-heading"><div className="section-toolbar"><div><h2 id="routes-heading">Routes</h2><span>{routes.length} configured</span></div><div className="toolbar-actions"><button className="button button--quiet" onClick={() => void exportRoutes()}>Export</button><button className="button button--quiet" onClick={() => importInput.current?.click()}>Import</button><input ref={importInput} className="sr-only" type="file" accept="application/json,.json" onChange={(event) => { const file = event.target.files?.[0]; if (file) void importRoutes(file) }} /><label className="search"><span className="sr-only">Search routes</span><input type="search" placeholder="Search domains or targets" value={query} onChange={(e) => setQuery(e.target.value)} /></label></div></div>
          {loading ? <div className="loading" aria-busy="true">Loading routes…</div> : <RouteTable routes={visible} zone={zone} onEdit={openEdit} onDelete={(route) => void remove(route)} onToggle={(route) => void toggle(route)} />}
        </section>
      </main>
      <footer><span>Local Reverse Proxy</span><span>Dashboard at 127.0.0.1:7400</span></footer>
      <RouteForm zone={zone} route={editing} open={formOpen} busy={saving} error={formError} onClose={() => setFormOpen(false)} onSave={save} />
    </div>
  )
}
