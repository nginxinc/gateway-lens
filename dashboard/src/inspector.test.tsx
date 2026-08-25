/*
Copyright 2026 F5, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import {render, screen} from '@testing-library/react'
import {describe, expect, it} from 'vitest'

import type {DashboardEdge, DashboardNode} from './types'
import {
  hasAttributes,
  InspectorList,
  InspectorSection,
  renderAttributes,
  renderConditions,
  renderRelationships,
} from './inspector'

function makeRef(kind: string, name: string, namespace?: string) {
  return {group: 'gateway.networking.k8s.io', kind, namespace, name}
}

function makeNode(
  kind: string,
  name: string,
  attrs: Record<string, string> = {},
): DashboardNode {
  return {ref: makeRef(kind, name, 'default'), attributes: attrs}
}

function makeEdge(
  fromKind: string,
  fromName: string,
  toKind: string,
  toName: string,
  detail: string,
): DashboardEdge {
  return {
    from: makeRef(fromKind, fromName, 'default'),
    to: makeRef(toKind, toName, 'default'),
    type: 'relationship',
    detail,
  }
}

// ---------------------------------------------------------------------------
// hasAttributes
// ---------------------------------------------------------------------------

describe('hasAttributes', () => {
  it('returns false for node with no attributes', () => {
    expect(hasAttributes(makeNode('Gateway', 'gw-1'))).toBe(false)
  })

  it('returns true for node with attributes', () => {
    expect(hasAttributes(makeNode('Gateway', 'gw-1', {port: '80'}))).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// InspectorSection
// ---------------------------------------------------------------------------

describe('InspectorSection', () => {
  it('renders title and children', () => {
    render(
      <InspectorSection title="Details">
        <p>content</p>
      </InspectorSection>,
    )
    expect(screen.getByText('Details')).toBeInTheDocument()
    expect(screen.getByText('content')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// InspectorList
// ---------------------------------------------------------------------------

describe('InspectorList', () => {
  it('renders label/value pairs', () => {
    render(
      <InspectorList
        items={[
          {label: 'Port', value: '80'},
          {label: 'Protocol', value: 'HTTP'},
        ]}
      />,
    )
    expect(screen.getByText('Port')).toBeInTheDocument()
    expect(screen.getByText('80')).toBeInTheDocument()
    expect(screen.getByText('Protocol')).toBeInTheDocument()
    expect(screen.getByText('HTTP')).toBeInTheDocument()
  })

  it('renders nothing for empty list', () => {
    const {container} = render(<InspectorList items={[]} />)
    expect(container.querySelector('.inspector-item')).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// renderAttributes
// ---------------------------------------------------------------------------

describe('renderAttributes', () => {
  it('renders attribute entries', () => {
    const node = makeNode('Gateway', 'gw-1', {port: '443', protocol: 'HTTPS'})
    render(<>{renderAttributes(node)}</>)
    expect(screen.getByText('port')).toBeInTheDocument()
    expect(screen.getByText('443')).toBeInTheDocument()
  })

  it('renders empty state for no attributes', () => {
    const node = makeNode('Gateway', 'gw-1')
    render(<>{renderAttributes(node)}</>)
    expect(screen.getByText(/no resource attributes/i)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// renderRelationships
// ---------------------------------------------------------------------------

describe('renderRelationships', () => {
  it('renders empty state when no edges', () => {
    const node = makeNode('Gateway', 'gw-1')
    render(<>{renderRelationships([], node)}</>)
    expect(screen.getByText(/no relationships/i)).toBeInTheDocument()
  })

  it('renders outgoing relationships', () => {
    const node = makeNode('Gateway', 'gw-1')
    const edges: DashboardEdge[] = [
      makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener'),
    ]
    render(<>{renderRelationships(edges, node)}</>)
    expect(screen.getByText('Outgoing')).toBeInTheDocument()
    expect(screen.getByText('HTTPRoute')).toBeInTheDocument()
    expect(screen.getByText('listener')).toBeInTheDocument()
  })

  it('renders incoming relationships', () => {
    const node = makeNode('HTTPRoute', 'route-1')
    const edges: DashboardEdge[] = [
      makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener'),
    ]
    render(<>{renderRelationships(edges, node)}</>)
    expect(screen.getByText('Incoming')).toBeInTheDocument()
    expect(screen.getByText('Gateway')).toBeInTheDocument()
  })

  it('renders both incoming and outgoing', () => {
    const node = makeNode('Gateway', 'gw-1')
    const edges: DashboardEdge[] = [
      makeEdge('GatewayClass', 'gc-1', 'Gateway', 'gw-1', 'gatewayClass'),
      makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener'),
    ]
    render(<>{renderRelationships(edges, node)}</>)
    expect(screen.getByText('Outgoing')).toBeInTheDocument()
    expect(screen.getByText('Incoming')).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// renderConditions
// ---------------------------------------------------------------------------

describe('renderConditions', () => {
  it('renders empty state for no conditions', () => {
    const node = makeNode('Gateway', 'gw-1')
    render(<>{renderConditions(node)}</>)
    expect(screen.getByText(/no gateway api conditions/i)).toBeInTheDocument()
  })

  it('renders condition type and status', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [{type: 'Programmed', status: 'True', reason: 'Programmed'}],
    }
    render(<>{renderConditions(node)}</>)
    expect(screen.getAllByText('Programmed')).toHaveLength(2) // type + reason
    expect(screen.getByText('True')).toBeInTheDocument()
  })

  it('renders reason and message when present', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [
        {
          type: 'Accepted',
          status: 'False',
          reason: 'Invalid',
          message: 'Listener port conflict',
        },
      ],
    }
    render(<>{renderConditions(node)}</>)
    expect(screen.getByText(/Invalid/)).toBeInTheDocument()
    expect(screen.getByText('Listener port conflict')).toBeInTheDocument()
  })

  it('renders multiple conditions', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [
        {type: 'Accepted', status: 'True'},
        {type: 'Programmed', status: 'True'},
      ],
    }
    render(<>{renderConditions(node)}</>)
    const cards = document.querySelectorAll('.condition-card')
    expect(cards).toHaveLength(2)
  })
})
