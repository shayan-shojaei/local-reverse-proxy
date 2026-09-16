import type { Route } from '../types'

interface Props {
  routes: Route[]
  zone: string
  onEdit: (route: Route) => void
  onDelete: (route: Route) => void
}

function Status({ route }: { route: Route }) {
  const label = !route.enabled ? 'Disabled' : route.health === 'reachable' ? 'Reachable' : route.health === 'unreachable' ? 'Unreachable' : 'Unchecked'
  return <span className={`status status--${route.enabled ? route.health : 'disabled'}`}><span aria-hidden="true" />{label}</span>
}

export function RouteTable({ routes, zone, onEdit, onDelete }: Props) {
  if (routes.length === 0) {
    return (
      <section className="empty-state" aria-labelledby="empty-heading">
        <div className="empty-state__mark" aria-hidden="true">↗</div>
        <h2 id="empty-heading">No routes yet</h2>
        <p>Create your first local domain and point it at a development server.</p>
      </section>
    )
  }

  return (
    <div className="table-wrap">
      <table>
        <thead><tr><th>Local domain</th><th>Upstream</th><th>Status</th><th><span className="sr-only">Actions</span></th></tr></thead>
        <tbody>
          {routes.map((route) => {
            const publicURL = `${route.publicMode}://${route.hostname}.${zone}`
            const upstreamURL = `${route.upstream.scheme}://${route.upstream.host}:${route.upstream.port}`
            return (
              <tr key={route.id}>
                <td data-label="Local domain">
                  <a className="domain-link" href={publicURL} target="_blank" rel="noreferrer">{publicURL}</a>
                  {route.publicMode === 'https' && <span className="meta">HTTP redirects to HTTPS</span>}
                </td>
                <td data-label="Upstream"><code>{upstreamURL}</code>{route.upstream.skipTlsVerify && <span className="warning">TLS verification off</span>}</td>
                <td data-label="Status"><Status route={route} /></td>
                <td className="row-actions">
                  <button className="button button--quiet" onClick={() => onEdit(route)}>Edit</button>
                  <button className="button button--danger-quiet" onClick={() => onDelete(route)}>Delete</button>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
