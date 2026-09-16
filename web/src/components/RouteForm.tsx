import { useEffect, useRef, useState } from 'react'
import type { Route, RouteDraft } from '../types'

interface Props {
  zone: string
  route: Route | null
  open: boolean
  busy: boolean
  error: string | null
  onClose: () => void
  onSave: (draft: RouteDraft) => Promise<void>
}

const emptyDraft: RouteDraft = {
  hostname: '', publicMode: 'https',
  upstream: { scheme: 'http', host: 'localhost', port: 3000, skipTlsVerify: false },
}

export function RouteForm({ zone, route, open, busy, error, onClose, onSave }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const [draft, setDraft] = useState<RouteDraft>(emptyDraft)

  useEffect(() => {
    setDraft(route ? { hostname: route.hostname, publicMode: route.publicMode, upstream: { ...route.upstream } } : emptyDraft)
  }, [route, open])
  useEffect(() => {
    if (open && !dialog.current?.open) dialog.current?.showModal()
    if (!open && dialog.current?.open) dialog.current.close()
  }, [open])

  const upstream = draft.upstream
  return (
    <dialog ref={dialog} onCancel={onClose} onClose={onClose} aria-labelledby="route-form-title">
      <form className="route-form" onSubmit={(event) => { event.preventDefault(); void onSave(draft) }}>
        <div className="form-heading">
          <div><p className="eyebrow">Route configuration</p><h2 id="route-form-title">{route ? 'Edit route' : 'Add a local domain'}</h2></div>
          <button type="button" className="icon-button" aria-label="Close" onClick={onClose}>×</button>
        </div>
        <div className="field">
          <label htmlFor="hostname">Local hostname</label>
          <div className="joined-input"><input id="hostname" required pattern="[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?" value={draft.hostname} onChange={(e) => setDraft({ ...draft, hostname: e.target.value.toLowerCase() })} autoFocus /><span>.{zone}</span></div>
          <small>Use letters, numbers, hyphens, and dots.</small>
        </div>
        <fieldset><legend>Public protocol</legend><div className="segmented">
          <label><input type="radio" name="publicMode" checked={draft.publicMode === 'https'} onChange={() => setDraft({ ...draft, publicMode: 'https' })} /><span>HTTPS <small>HTTP redirects</small></span></label>
          <label><input type="radio" name="publicMode" checked={draft.publicMode === 'http'} onChange={() => setDraft({ ...draft, publicMode: 'http' })} /><span>HTTP only</span></label>
        </div></fieldset>
        <fieldset><legend>Upstream application</legend><div className="upstream-grid">
          <div className="field"><label htmlFor="scheme">Scheme</label><select id="scheme" value={upstream.scheme} onChange={(e) => setDraft({ ...draft, upstream: { ...upstream, scheme: e.target.value as 'http' | 'https', skipTlsVerify: false } })}><option>http</option><option>https</option></select></div>
          <div className="field field--host"><label htmlFor="host">Hostname or IP</label><input id="host" required value={upstream.host} onChange={(e) => setDraft({ ...draft, upstream: { ...upstream, host: e.target.value } })} /></div>
          <div className="field"><label htmlFor="port">Port</label><input id="port" type="number" min="1" max="65535" required value={upstream.port} onChange={(e) => setDraft({ ...draft, upstream: { ...upstream, port: Number(e.target.value) } })} /></div>
        </div></fieldset>
        {upstream.scheme === 'https' && <label className="check"><input type="checkbox" checked={upstream.skipTlsVerify} onChange={(e) => setDraft({ ...draft, upstream: { ...upstream, skipTlsVerify: e.target.checked } })} /><span><strong>Skip upstream TLS verification</strong><small>Use only for a local server with an untrusted certificate.</small></span></label>}
        {error && <div className="form-error" role="alert">{error}</div>}
        <div className="form-actions"><button type="button" className="button button--secondary" onClick={onClose}>Cancel</button><button className="button button--primary" disabled={busy}>{busy ? 'Applying…' : route ? 'Save changes' : 'Create route'}</button></div>
      </form>
    </dialog>
  )
}
