import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { RouteTable } from './RouteTable'

it('shows a useful empty state', () => {
  render(<RouteTable routes={[]} zone="local.test" onEdit={() => undefined} onDelete={() => undefined} />)
  expect(screen.getByRole('heading', { name: /no routes yet/i })).toBeInTheDocument()
  expect(screen.getByText(/first local domain/i)).toBeInTheDocument()
})

it('renders a route with its complete public URL and upstream', () => {
  render(
    <RouteTable
      zone="local.test"
      routes={[{
        id: '1', hostname: 'api', enabled: true, publicMode: 'https', revision: 1,
        health: 'reachable', createdAt: '', updatedAt: '',
        upstream: { scheme: 'http', host: 'localhost', port: 3000, skipTlsVerify: false },
      }]}
      onEdit={() => undefined}
      onDelete={() => undefined}
    />,
  )
  expect(screen.getByText('https://api.local.test')).toBeInTheDocument()
  expect(screen.getByText('http://localhost:3000')).toBeInTheDocument()
  expect(screen.getByText('Reachable')).toBeInTheDocument()
})
