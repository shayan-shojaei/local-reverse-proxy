import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from './api'
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

  const load = useCallback(async () => {
    try {
      const [nextRoutes, nextStatus] = await Promise.all([api.listRoutes(), api.status()])
      setRoutes(nextRoutes); setStatus(nextStatus); setPageError(null)
    } catch (error) { setPageError(error instanceof Error ? error.message : 'Could not load the dashboard') }
    finally { setLoading(false) }
  }, [])
  useEffect(() => { void load() }, [load])

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

  const zone = status?.zone ?? 'local.test'
  return (
    <div className="app-shell">
      <header className="topbar"><a className="brand" href="/" aria-label="Local Reverse Proxy home"><span className="brand__mark">LRP</span><span>Local Reverse Proxy</span></a><span className="local-badge">Local only</span></header>
      <main>
        <section className="page-heading"><div><p className="eyebrow">Domain routing</p><h1>Your local apps, properly addressed.</h1><p>Give every development server a memorable <code>.test</code> URL with trusted HTTPS.</p></div><button className="button button--primary button--large" onClick={openCreate}>＋ Add route</button></section>
        <section className="system-strip" aria-label="System status"><div><span className={`health-dot ${status ? '' : 'health-dot--muted'}`} /><span><strong>{status ? 'Proxy services online' : 'Checking services…'}</strong><small>{zone} · loopback only</small></span></div><button className="button button--quiet" onClick={() => void load()}>Run checks</button></section>
        {pageError && <div className="page-error" role="alert"><span>{pageError}</span><button onClick={() => void load()}>Retry</button></div>}
        <section className="routes-section" aria-labelledby="routes-heading"><div className="section-toolbar"><div><h2 id="routes-heading">Routes</h2><span>{routes.length} configured</span></div><label className="search"><span className="sr-only">Search routes</span><input type="search" placeholder="Search domains or targets" value={query} onChange={(e) => setQuery(e.target.value)} /></label></div>
          {loading ? <div className="loading" aria-busy="true">Loading routes…</div> : <RouteTable routes={visible} zone={zone} onEdit={openEdit} onDelete={(route) => void remove(route)} />}
        </section>
      </main>
      <footer><span>Local Reverse Proxy</span><span>Dashboard at 127.0.0.1:7400</span></footer>
      <RouteForm zone={zone} route={editing} open={formOpen} busy={saving} error={formError} onClose={() => setFormOpen(false)} onSave={save} />
    </div>
  )
}
