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

import {describe, expect, it, vi} from 'vitest'

import type {DashboardEdge, DashboardNode, DashboardPayload, DashboardResourceRef} from '../types'
import type {Node} from '@xyflow/react'

import type {GroupRect} from './graph'
import {
  apiURL,
  applyCollapsing,
  applyEdgeHover,
  applyFilters,
  applyNamespaceGrouping,
  buildGraph,
  buildSelectedResourceYAML,
  edgeKey,
  formatResource,
  groupNodeCollapseThreshold,
  groupNodeKey,
  isGroupNodeKey,
  isNamespaceGroupNodeId,
  matchesSearch,
  namespaceGroupNodeId,
  nodeHasNegativeCondition,
  resolveGroupOverlaps,
  resolveSelectedKey,
  resourceKey,
  visibleGraphSnapshot,
} from './graph'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeRef(
  kind: string,
  name: string,
  namespace?: string,
  group = 'gateway.networking.k8s.io',
): DashboardResourceRef {
  return {group, kind, namespace, name}
}

function makeNode(kind: string, name: string, namespace?: string, group?: string): DashboardNode {
  return {ref: makeRef(kind, name, namespace, group), attributes: {}}
}

function makeEdge(
  fromKind: string,
  fromName: string,
  toKind: string,
  toName: string,
  detail: string,
  namespace?: string,
): DashboardEdge {
  return {
    from: makeRef(fromKind, fromName, namespace),
    to: makeRef(toKind, toName, namespace),
    type: 'relationship',
    detail,
  }
}

function makePayload(
  nodes: DashboardNode[],
  edges: DashboardEdge[] = [],
  annotations: DashboardPayload['annotations'] = [],
): DashboardPayload {
  return {generatedAt: new Date().toISOString(), nodes, edges, annotations}
}

// ---------------------------------------------------------------------------
// resourceKey / edgeKey / formatResource
// ---------------------------------------------------------------------------

describe('resourceKey', () => {
  it('joins all ref parts with pipe', () => {
    expect(resourceKey(makeRef('Gateway', 'gw-1', 'default'))).toBe(
      'gateway.networking.k8s.io|Gateway|default|gw-1',
    )
  })

  it('handles missing namespace', () => {
    expect(resourceKey(makeRef('GatewayClass', 'gc-1'))).toBe(
      'gateway.networking.k8s.io|GatewayClass||gc-1',
    )
  })
})

describe('edgeKey', () => {
  it('combines from→to with type and detail', () => {
    const edge = makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener')
    const key = edgeKey(edge)
    expect(key).toContain('->')
    expect(key).toContain('listener')
    expect(key).toContain('relationship')
  })
})

describe('formatResource', () => {
  it('includes namespace when present', () => {
    expect(formatResource(makeRef('Gateway', 'gw-1', 'default'))).toBe('default/gw-1')
  })

  it('omits namespace when absent', () => {
    expect(formatResource(makeRef('GatewayClass', 'gc-1'))).toBe('gc-1')
  })
})

// ---------------------------------------------------------------------------
// groupNodeKey / isGroupNodeKey
// ---------------------------------------------------------------------------

describe('groupNodeKey', () => {
  it('prefixes kind with group marker', () => {
    const key = groupNodeKey('HTTPRoute')
    expect(key).toContain('HTTPRoute')
    expect(isGroupNodeKey(key)).toBe(true)
  })
})

describe('isGroupNodeKey', () => {
  it('returns false for regular names', () => {
    expect(isGroupNodeKey('my-route')).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// nodeHasNegativeCondition
// ---------------------------------------------------------------------------

describe('nodeHasNegativeCondition', () => {
  it('returns false for node with no conditions', () => {
    expect(nodeHasNegativeCondition(makeNode('Gateway', 'gw-1'))).toBe(false)
  })

  it('returns false when conditions are all True', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [
        {type: 'Accepted', status: 'True'},
        {type: 'Programmed', status: 'True'},
      ],
    }
    expect(nodeHasNegativeCondition(node)).toBe(false)
  })

  it('returns true when Accepted is False', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [{type: 'Accepted', status: 'False'}],
    }
    expect(nodeHasNegativeCondition(node)).toBe(true)
  })

  it('returns true when Programmed is False', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [{type: 'Programmed', status: 'False'}],
    }
    expect(nodeHasNegativeCondition(node)).toBe(true)
  })

  it('returns true when ResolvedRefs is False', () => {
    const node: DashboardNode = {
      ...makeNode('HTTPRoute', 'route-1'),
      conditions: [{type: 'ResolvedRefs', status: 'False'}],
    }
    expect(nodeHasNegativeCondition(node)).toBe(true)
  })

  it('handles prefixed condition types (e.g. listener/http/Accepted)', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [{type: 'listener/http/Accepted', status: 'False'}],
    }
    expect(nodeHasNegativeCondition(node)).toBe(true)
  })

  it('ignores non-error condition types', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [{type: 'CustomCondition', status: 'False'}],
    }
    expect(nodeHasNegativeCondition(node)).toBe(false)
  })

  it('is case-insensitive on status', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      conditions: [{type: 'Accepted', status: 'false'}],
    }
    expect(nodeHasNegativeCondition(node)).toBe(true)
  })

  it('returns true when an Error-severity diagnostic is present', () => {
    const node: DashboardNode = {
      ...makeNode('Service', 'svc-1'),
      diagnostics: [{severity: 'Error', reason: 'NoReadyEndpoints', message: 'The Service has no ready endpoints.'}],
    }
    expect(nodeHasNegativeCondition(node)).toBe(true)
  })

  it('returns false when only Info-severity diagnostics are present', () => {
    const node: DashboardNode = {
      ...makeNode('Service', 'svc-1'),
      diagnostics: [{severity: 'Info', reason: 'SomeInfo', message: 'Just some info.'}],
    }
    expect(nodeHasNegativeCondition(node)).toBe(false)
  })

  it('returns false when node has no diagnostics', () => {
    const node: DashboardNode = {
      ...makeNode('Service', 'svc-1'),
      diagnostics: [],
    }
    expect(nodeHasNegativeCondition(node)).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// matchesSearch
// ---------------------------------------------------------------------------

describe('matchesSearch', () => {
  const ref = makeRef('HTTPRoute', 'route-1', 'ns-a')

  it('returns true for an empty search', () => {
    expect(matchesSearch(ref, '')).toBe(true)
    expect(matchesSearch(ref, '   ')).toBe(true)
  })

  it('matches by name (case-insensitive) when there is no slash', () => {
    expect(matchesSearch(ref, 'ROUTE-1')).toBe(true)
    expect(matchesSearch(ref, 'nope')).toBe(false)
  })

  it('matches by namespace (case-insensitive) when there is no slash', () => {
    expect(matchesSearch(ref, 'ns-a')).toBe(true)
    expect(matchesSearch(ref, 'NS-A')).toBe(true)
  })

  it('does not match kind when there is no slash', () => {
    expect(matchesSearch(ref, 'httproute')).toBe(false)
  })

  it('supports "namespace/name" syntax requiring both to match', () => {
    expect(matchesSearch(ref, 'ns-a/route-1')).toBe(true)
    expect(matchesSearch(ref, 'ns-a/nope')).toBe(false)
    expect(matchesSearch(ref, 'wrong-ns/route-1')).toBe(false)
  })

  it('allows leaving one side of the slash blank', () => {
    // "namespace/" matches any name in that namespace.
    expect(matchesSearch(ref, 'ns-a/')).toBe(true)
    expect(matchesSearch(ref, 'wrong-ns/')).toBe(false)
    // "/name" matches that name regardless of namespace.
    expect(matchesSearch(ref, '/route-1')).toBe(true)
    expect(matchesSearch(ref, '/nope')).toBe(false)
  })

  it('trims whitespace around each side of the slash', () => {
    expect(matchesSearch(ref, ' ns-a / route-1 ')).toBe(true)
  })

  it('handles a resource with no namespace', () => {
    const clusterScoped = makeRef('GatewayClass', 'gc-1', undefined)
    expect(matchesSearch(clusterScoped, 'gc-1')).toBe(true)
    expect(matchesSearch(clusterScoped, '/gc-1')).toBe(true)
    expect(matchesSearch(clusterScoped, 'default/gc-1')).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// applyFilters
// ---------------------------------------------------------------------------

describe('applyFilters', () => {
  const snapshot = makePayload(
    [
      makeNode('GatewayClass', 'gc-1'),
      makeNode('Gateway', 'gw-1', 'ns-a'),
      makeNode('Gateway', 'gw-2', 'ns-b'),
      makeNode('HTTPRoute', 'route-1', 'ns-a'),
    ],
    [
      makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener', 'ns-a'),
    ],
    [{ref: makeRef('Gateway', 'gw-1', 'ns-a'), source: 'test', key: 'k', value: 'v'}],
  )

  it('returns everything when no filters applied', () => {
    const result = applyFilters(snapshot, new Set(), '', '')
    expect(result.nodes).toHaveLength(4)
    expect(result.edges).toHaveLength(1)
  })

  it('filters by hidden kinds', () => {
    const result = applyFilters(snapshot, new Set(['HTTPRoute']), '', '')
    expect(result.nodes).toHaveLength(3)
    expect(result.nodes.every((n) => n.ref.kind !== 'HTTPRoute')).toBe(true)
    // Edge to HTTPRoute should be removed since one endpoint is gone
    expect(result.edges).toHaveLength(0)
  })

  it('filters by namespace', () => {
    const result = applyFilters(snapshot, new Set(), 'ns-a', '')
    const names = result.nodes.map((n) => n.ref.name)
    expect(names).toContain('gw-1')
    expect(names).toContain('route-1')
    expect(names).not.toContain('gw-2')
    // GatewayClass has no namespace, filtered out
    expect(names).not.toContain('gc-1')
  })

  it('filters by search string (case-insensitive)', () => {
    const result = applyFilters(snapshot, new Set(), '', 'ROUTE')
    expect(result.nodes).toHaveLength(1)
    expect(result.nodes[0].ref.name).toBe('route-1')
  })

  it('filters by search string matching namespace', () => {
    const result = applyFilters(snapshot, new Set(), '', 'ns-b')
    expect(result.nodes).toHaveLength(1)
    expect(result.nodes[0].ref.name).toBe('gw-2')
  })

  it('does not match search strings against kind', () => {
    const result = applyFilters(snapshot, new Set(), '', 'gatewayclass')
    expect(result.nodes).toHaveLength(0)
  })

  it('filters by "namespace/name" syntax', () => {
    const result = applyFilters(snapshot, new Set(), '', 'ns-a/gw-1')
    expect(result.nodes).toHaveLength(1)
    expect(result.nodes[0].ref.name).toBe('gw-1')
  })

  it('filters annotations along with nodes', () => {
    const result = applyFilters(snapshot, new Set(['Gateway']), '', '')
    expect(result.annotations).toHaveLength(0)
  })

  it('combines filters', () => {
    const result = applyFilters(snapshot, new Set(['GatewayClass']), 'ns-a', 'gw')
    expect(result.nodes).toHaveLength(1)
    expect(result.nodes[0].ref.name).toBe('gw-1')
  })
})

// ---------------------------------------------------------------------------
// applyCollapsing
// ---------------------------------------------------------------------------

describe('applyCollapsing', () => {
  it('returns snapshot unchanged when no kinds collapsed', () => {
    const snapshot = makePayload([makeNode('Gateway', 'gw-1')])
    const result = applyCollapsing(snapshot, new Set())
    expect(result).toBe(snapshot) // same reference, not a copy
  })

  it('collapses nodes of a kind into a single group node', () => {
    const nodes = Array.from({length: 10}, (_, i) =>
      makeNode('HTTPRoute', `route-${i}`, 'default'),
    )
    const snapshot = makePayload(nodes)
    const result = applyCollapsing(snapshot, new Set(['HTTPRoute']))

    const routeNodes = result.nodes.filter((n) => n.ref.kind === 'HTTPRoute')
    expect(routeNodes).toHaveLength(1)
    expect(isGroupNodeKey(routeNodes[0].ref.name)).toBe(true)
    expect(routeNodes[0].attributes['__count']).toBe('10')
  })

  it('redirects edges to group nodes', () => {
    const snapshot = makePayload(
      [
        makeNode('Gateway', 'gw-1', 'default'),
        makeNode('HTTPRoute', 'route-1', 'default'),
        makeNode('HTTPRoute', 'route-2', 'default'),
      ],
      [
        makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener', 'default'),
        makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-2', 'listener', 'default'),
      ],
    )

    const result = applyCollapsing(snapshot, new Set(['HTTPRoute']))
    // Two edges from gw-1→route-1 and gw-1→route-2 should be deduplicated into one gw-1→group
    expect(result.edges).toHaveLength(1)
    expect(isGroupNodeKey(result.edges[0].to.name)).toBe(true)
  })

  it('preserves non-collapsed nodes', () => {
    const snapshot = makePayload([
      makeNode('Gateway', 'gw-1'),
      makeNode('HTTPRoute', 'route-1'),
    ])
    const result = applyCollapsing(snapshot, new Set(['HTTPRoute']))
    expect(result.nodes.some((n) => n.ref.kind === 'Gateway' && n.ref.name === 'gw-1')).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// visibleGraphSnapshot
// ---------------------------------------------------------------------------

describe('visibleGraphSnapshot', () => {
  it('filters out synthetic wildcard routes', () => {
    const snapshot = makePayload([
      makeNode('Gateway', 'gw-1'),
      makeNode('HTTPRoute', '*'),
    ])
    const result = visibleGraphSnapshot(snapshot)
    expect(result.nodes).toHaveLength(1)
    expect(result.nodes[0].ref.kind).toBe('Gateway')
  })

  it('does not filter non-wildcard routes', () => {
    const snapshot = makePayload([makeNode('HTTPRoute', 'real-route')])
    const result = visibleGraphSnapshot(snapshot)
    expect(result.nodes).toHaveLength(1)
  })

  it('enriches ReferenceGrant nodes with summary attribute', () => {
    const rg = makeNode('ReferenceGrant', 'rg-1', 'backend')
    const gw = makeNode('Gateway', 'gw-1', 'frontend')
    const svc = makeNode('Service', 'svc-1', 'backend')

    const edges: DashboardEdge[] = [
      {
        from: makeRef('HTTPRoute', 'route-1', 'frontend'),
        to: makeRef('ReferenceGrant', 'rg-1', 'backend'),
        type: 'relationship',
        detail: 'referenceGrantFrom',
      },
      {
        from: makeRef('ReferenceGrant', 'rg-1', 'backend'),
        to: makeRef('Service', 'svc-1', 'backend'),
        type: 'relationship',
        detail: 'referenceGrantTo',
      },
    ]

    const snapshot = makePayload([rg, gw, svc], edges)
    const result = visibleGraphSnapshot(snapshot)

    const rgNode = result.nodes.find((n) => n.ref.kind === 'ReferenceGrant')!
    expect(rgNode.attributes['ReferenceGrant Summary']).toContain('frontend namespace')
    expect(rgNode.attributes['ReferenceGrant Summary']).toContain('backend/svc-1')
  })

  it('filters edges whose endpoints were removed', () => {
    const snapshot = makePayload(
      [makeNode('Gateway', 'gw-1'), makeNode('HTTPRoute', '*')],
      [makeEdge('Gateway', 'gw-1', 'HTTPRoute', '*', 'listener')],
    )
    const result = visibleGraphSnapshot(snapshot)
    expect(result.edges).toHaveLength(0)
  })
})

// ---------------------------------------------------------------------------
// resolveSelectedKey
// ---------------------------------------------------------------------------

describe('resolveSelectedKey', () => {
  it('returns empty string for empty snapshot', () => {
    expect(resolveSelectedKey(makePayload([]), 'anything')).toBe('')
  })

  it('keeps current key if it still exists in snapshot', () => {
    const node = makeNode('Gateway', 'gw-1')
    const key = resourceKey(node.ref)
    expect(resolveSelectedKey(makePayload([node]), key)).toBe(key)
  })

  it('clears selection if current key no longer exists in snapshot', () => {
    const node = makeNode('Gateway', 'gw-1')
    const result = resolveSelectedKey(makePayload([node]), 'nonexistent')
    expect(result).toBe('')
  })

  it('never auto-selects when nothing was previously selected (deselect persists)', () => {
    const node = makeNode('Gateway', 'gw-1')
    expect(resolveSelectedKey(makePayload([node]), '')).toBe('')
  })
})

// ---------------------------------------------------------------------------
// buildSelectedResourceYAML
// ---------------------------------------------------------------------------

describe('buildSelectedResourceYAML', () => {
  it('returns manifest when present', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1'),
      manifest: 'apiVersion: v1\nkind: Gateway',
    }
    expect(buildSelectedResourceYAML(node)).toBe('apiVersion: v1\nkind: Gateway')
  })

  it('returns fallback when no manifest', () => {
    expect(buildSelectedResourceYAML(makeNode('Gateway', 'gw-1'))).toContain('unavailable')
  })
})

// ---------------------------------------------------------------------------
// apiURL
// ---------------------------------------------------------------------------

describe('apiURL', () => {
  it('strips the leading slash so the path resolves relative to the current document', () => {
    // import.meta.env.VITE_API_BASE_URL is undefined in test, so base is ""
    expect(apiURL('/api/graph')).toBe('api/graph')
  })

  it('leaves pathnames without a leading slash unchanged', () => {
    expect(apiURL('data')).toBe('data')
  })

  it('prefixes with VITE_API_BASE_URL when configured', () => {
    vi.stubEnv('VITE_API_BASE_URL', 'http://localhost:8080')

    expect(apiURL('/data')).toBe('http://localhost:8080/data')

    vi.unstubAllEnvs()
  })
})

// ---------------------------------------------------------------------------
// buildGraph
// ---------------------------------------------------------------------------

describe('buildGraph', () => {
  it('produces nodes and edges for a basic snapshot', () => {
    // Edge refs must exactly match node refs (same namespace) for dagre layout to work.
    const gc = makeNode('GatewayClass', 'gc-1')
    const gw = makeNode('Gateway', 'gw-1', 'default')
    const edge: DashboardEdge = {
      from: gw.ref,
      to: gc.ref,
      type: 'relationship',
      detail: 'gatewayClass',
    }
    const snapshot = makePayload([gc, gw], [edge])

    const graph = buildGraph(snapshot, '')
    expect(graph.nodes).toHaveLength(2)
    expect(graph.edges).toHaveLength(1)
  })

  it('assigns position to each node', () => {
    const snapshot = makePayload([makeNode('Gateway', 'gw-1', 'default')])
    const graph = buildGraph(snapshot, '')

    expect(graph.nodes[0].position.x).toBeTypeOf('number')
    expect(graph.nodes[0].position.y).toBeTypeOf('number')
  })

  it('marks a node as selected', () => {
    const node = makeNode('Gateway', 'gw-1', 'default')
    const key = resourceKey(node.ref)
    const snapshot = makePayload([node])

    const graph = buildGraph(snapshot, key)
    const flowNode = graph.nodes[0]

    // Selected node should have selectedFill background
    expect(flowNode.style?.background).toContain('1)')
  })

  it('applies a distinct selection glow ring to the selected node only', () => {
    const selectedNode = makeNode('Gateway', 'gw-1', 'default')
    const otherNode = makeNode('Gateway', 'gw-2', 'default')
    const key = resourceKey(selectedNode.ref)
    const snapshot = makePayload([selectedNode, otherNode])

    const graph = buildGraph(snapshot, key)
    const selectedFlowNode = graph.nodes.find((n) => n.id === key)
    const otherFlowNode = graph.nodes.find((n) => n.id !== key)

    expect(String(selectedFlowNode?.style?.boxShadow)).toContain('37, 99, 235')
    expect(String(otherFlowNode?.style?.boxShadow)).not.toContain('37, 99, 235')
  })

  it('shows both the selection glow and the error glow when a selected node has an error', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1', 'default'),
      conditions: [{type: 'Accepted', status: 'False'}],
    }
    const key = resourceKey(node.ref)
    const snapshot = makePayload([node])

    const graph = buildGraph(snapshot, key)
    const boxShadow = String(graph.nodes[0].style?.boxShadow)

    expect(boxShadow).toContain('37, 99, 235')
    expect(boxShadow).toContain('193, 49, 38')
  })

  it('creates group-type nodes for group keys', () => {
    const groupNode: DashboardNode = {
      ref: makeRef('HTTPRoute', groupNodeKey('HTTPRoute'), ''),
      attributes: {'__count': '5'},
    }
    const snapshot = makePayload([groupNode])
    const graph = buildGraph(snapshot, '')

    expect(graph.nodes[0].type).toBe('group')
    expect(graph.nodes[0].data.count).toBe(5)
  })

  it('creates resource-type nodes for regular nodes', () => {
    const snapshot = makePayload([makeNode('Gateway', 'gw-1', 'default')])
    const graph = buildGraph(snapshot, '')
    expect(graph.nodes[0].type).toBe('resource')
  })

  it('sets error styling on nodes with negative conditions', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1', 'default'),
      conditions: [{type: 'Accepted', status: 'False'}],
    }
    const snapshot = makePayload([node])
    const graph = buildGraph(snapshot, '')

    // Error node should have a wider border
    expect(flowNodeBorderWidth(graph.nodes[0])).toBeGreaterThan(1)
  })

  it('includes a readiness badge for a Service node with endpoint attributes', () => {
    const node: DashboardNode = {
      ...makeNode('Service', 'svc-1', 'default', ''),
      attributes: {readyEndpoints: '2', totalEndpoints: '3'},
    }
    const snapshot = makePayload([node])
    const graph = buildGraph(snapshot, '')

    expect(graph.nodes[0].data.readinessBadge).toBe('2/3 ready')
  })

  it('omits the readiness badge for non-Service nodes', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1', 'default'),
      attributes: {readyEndpoints: '2', totalEndpoints: '3'},
    }
    const snapshot = makePayload([node])
    const graph = buildGraph(snapshot, '')

    expect(graph.nodes[0].data.readinessBadge).toBeUndefined()
  })

  it('applies error styling to a Service node with zero ready endpoints', () => {
    const node: DashboardNode = {
      ...makeNode('Service', 'svc-1', 'default', ''),
      attributes: {readyEndpoints: '0', totalEndpoints: '3'},
      diagnostics: [{severity: 'Error', reason: 'NoReadyEndpoints', message: 'The Service has no ready endpoints.'}],
    }
    const snapshot = makePayload([node])
    const graph = buildGraph(snapshot, '')

    expect(graph.nodes[0].data.readinessBadge).toBe('0/3 ready')
    expect(graph.nodes[0].data.hasNegativeCondition).toBe(true)
    expect(flowNodeBorderWidth(graph.nodes[0])).toBeGreaterThan(1)
  })

  it('sets smoothstep edge type', () => {
    const snapshot = makePayload(
      [makeNode('Gateway', 'gw-1', 'default'), makeNode('HTTPRoute', 'route-1', 'default')],
      [makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener', 'default')],
    )
    const graph = buildGraph(snapshot, '')
    expect(graph.edges[0].type).toBe('smoothstep')
  })

  it('disables dragging on all nodes', () => {
    const snapshot = makePayload([makeNode('Gateway', 'gw-1', 'default')])
    const graph = buildGraph(snapshot, '')
    expect(graph.nodes[0].draggable).toBe(false)
  })

  it('creates namespace group nodes when multiple namespaces are present', () => {
    const gw = makeNode('Gateway', 'gw-1', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const edge: DashboardEdge = {
      from: gw.ref,
      to: route.ref,
      type: 'relationship',
      detail: 'parentRef',
    }
    const snapshot = makePayload([gw, route], [edge])
    const graph = buildGraph(snapshot, '')

    const nsGroupNodes = graph.nodes.filter((n) => n.type === 'namespaceGroup')
    expect(nsGroupNodes).toHaveLength(2)

    // Child nodes should have parentId set.
    const childNodes = graph.nodes.filter((n) => n.type === 'resource')
    expect(childNodes).toHaveLength(2)
    for (const child of childNodes) {
      expect(child.parentId).toBeDefined()
      expect(isNamespaceGroupNodeId(child.parentId!)).toBe(true)
    }
  })

  it('skips namespace grouping with a single namespace', () => {
    const snapshot = makePayload([
      makeNode('Gateway', 'gw-1', 'default'),
      makeNode('HTTPRoute', 'route-1', 'default'),
    ])
    const graph = buildGraph(snapshot, '')

    const nsGroupNodes = graph.nodes.filter((n) => n.type === 'namespaceGroup')
    expect(nsGroupNodes).toHaveLength(0)
  })

  it('defaults to the light scheme when none is provided', () => {
    const snapshot = makePayload([makeNode('Gateway', 'gw-1', 'default')])
    expect(buildGraph(snapshot, '')).toEqual(buildGraph(snapshot, '', 'light'))
  })

  it('applies dark-scheme colors to node fill/stroke and edge labels', () => {
    const gw = makeNode('Gateway', 'gw-1', 'default')
    const route = makeNode('HTTPRoute', 'route-1', 'default')
    const edge = makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener', 'default')
    const snapshot = makePayload([gw, route], [edge])

    const lightGraph = buildGraph(snapshot, '', 'light')
    const darkGraph = buildGraph(snapshot, '', 'dark')

    expect(darkGraph.nodes[0].style?.background).not.toEqual(lightGraph.nodes[0].style?.background)
    expect(darkGraph.edges[0].labelStyle).not.toEqual(lightGraph.edges[0].labelStyle)
    expect(darkGraph.edges[0].labelBgStyle).not.toEqual(lightGraph.edges[0].labelBgStyle)
  })

  it('applies a distinct dark-scheme error glow color', () => {
    const node: DashboardNode = {
      ...makeNode('Gateway', 'gw-1', 'default'),
      conditions: [{type: 'Accepted', status: 'False'}],
    }
    const snapshot = makePayload([node])

    const lightBoxShadow = String(buildGraph(snapshot, '', 'light').nodes[0].style?.boxShadow)
    const darkBoxShadow = String(buildGraph(snapshot, '', 'dark').nodes[0].style?.boxShadow)

    expect(lightBoxShadow).toContain('193, 49, 38')
    expect(darkBoxShadow).not.toContain('193, 49, 38')
    expect(darkBoxShadow).toContain('255, 120, 105')
  })

  it('keeps the selection ring color identical across schemes', () => {
    const node = makeNode('Gateway', 'gw-1', 'default')
    const key = resourceKey(node.ref)
    const snapshot = makePayload([node])

    const lightBoxShadow = String(buildGraph(snapshot, key, 'light').nodes[0].style?.boxShadow)
    const darkBoxShadow = String(buildGraph(snapshot, key, 'dark').nodes[0].style?.boxShadow)

    expect(lightBoxShadow).toContain('37, 99, 235')
    expect(darkBoxShadow).toContain('37, 99, 235')
  })

  it('defaults to top-to-bottom orientation when none is provided', () => {
    const snapshot = makePayload([
      makeNode('Gateway', 'gw-1', 'default'),
      makeNode('HTTPRoute', 'route-1', 'default'),
    ])
    expect(buildGraph(snapshot, '', 'light')).toEqual(buildGraph(snapshot, '', 'light', 'TB'))
  })

  it('lays out nodes left-to-right when orientation is LR', () => {
    const gw = makeNode('Gateway', 'gw-1', 'default')
    const route = makeNode('HTTPRoute', 'route-1', 'default')
    const edge = makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener', 'default')
    const snapshot = makePayload([gw, route], [edge])

    const tbGraph = buildGraph(snapshot, '', 'light', 'TB')
    const lrGraph = buildGraph(snapshot, '', 'light', 'LR')

    const tbGw = tbGraph.nodes.find((n) => n.id === resourceKey(gw.ref))!
    const tbRoute = tbGraph.nodes.find((n) => n.id === resourceKey(route.ref))!
    const lrGw = lrGraph.nodes.find((n) => n.id === resourceKey(gw.ref))!
    const lrRoute = lrGraph.nodes.find((n) => n.id === resourceKey(route.ref))!

    // In TB layout, ranked nodes are separated primarily along the y-axis.
    expect(tbGw.position.y).not.toEqual(tbRoute.position.y)
    // In LR layout, ranked nodes are separated primarily along the x-axis.
    expect(lrGw.position.x).not.toEqual(lrRoute.position.x)
  })

  it('assigns left/right handle sides to edges in LR orientation', () => {
    const gw = makeNode('Gateway', 'gw-1', 'default')
    const route = makeNode('HTTPRoute', 'route-1', 'default')
    const edge = makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener', 'default')
    const snapshot = makePayload([gw, route], [edge])

    const graph = buildGraph(snapshot, '', 'light', 'LR')
    const flowEdge = graph.edges[0]

    expect(String(flowEdge.sourceHandle)).toContain('right')
    expect(String(flowEdge.targetHandle)).toContain('left')
  })

  it('populates left/right handle counts on resource node data in LR orientation', () => {
    const gw = makeNode('Gateway', 'gw-1', 'default')
    const route = makeNode('HTTPRoute', 'route-1', 'default')
    const edge = makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-1', 'listener', 'default')
    const snapshot = makePayload([gw, route], [edge])

    const graph = buildGraph(snapshot, '', 'light', 'LR')
    const gwNode = graph.nodes.find((n) => n.id === resourceKey(gw.ref))!
    const routeNode = graph.nodes.find((n) => n.id === resourceKey(route.ref))!

    expect(gwNode.data.sourceRightHandleCount).toBe(1)
    expect(gwNode.data.sourceTopHandleCount).toBe(0)
    expect(gwNode.data.sourceBottomHandleCount).toBe(0)
    expect(routeNode.data.targetLeftHandleCount).toBe(1)
    expect(routeNode.data.targetTopHandleCount).toBe(0)
    expect(routeNode.data.targetBottomHandleCount).toBe(0)
  })

  it('keeps namespace grouping correct under LR orientation', () => {
    const gw = makeNode('Gateway', 'gw-1', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const edge: DashboardEdge = {
      from: gw.ref,
      to: route.ref,
      type: 'relationship',
      detail: 'parentRef',
    }
    const snapshot = makePayload([gw, route], [edge])
    const graph = buildGraph(snapshot, '', 'light', 'LR')

    const nsGroupNodes = graph.nodes.filter((n) => n.type === 'namespaceGroup')
    expect(nsGroupNodes).toHaveLength(2)

    const childNodes = graph.nodes.filter((n) => n.type === 'resource')
    expect(childNodes).toHaveLength(2)
    for (const child of childNodes) {
      expect(child.parentId).toBeDefined()
      expect(isNamespaceGroupNodeId(child.parentId!)).toBe(true)
      // Child position must stay within its namespace group's bounds.
      const parent = nsGroupNodes.find((n) => n.id === child.parentId)!
      expect(child.position.x).toBeGreaterThanOrEqual(0)
      expect(child.position.y).toBeGreaterThanOrEqual(0)
      expect(parent.style?.width).toBeTypeOf('number')
    }
  })
})

// ---------------------------------------------------------------------------
// applyEdgeHover
// ---------------------------------------------------------------------------

describe('applyEdgeHover', () => {
  function buildTwoEdgeGraph() {
    const gw = makeNode('Gateway', 'gw-1', 'default')
    const routeA = makeNode('HTTPRoute', 'route-a', 'default')
    const routeB = makeNode('HTTPRoute', 'route-b', 'default')
    const edgeA = makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-a', 'listener', 'default')
    const edgeB = makeEdge('Gateway', 'gw-1', 'HTTPRoute', 'route-b', 'listener', 'default')
    const snapshot = makePayload([gw, routeA, routeB], [edgeA, edgeB])
    return buildGraph(snapshot, '', 'light')
  }

  it('returns the same array when no key is hovered', () => {
    const graph = buildTwoEdgeGraph()
    expect(applyEdgeHover(graph.edges, '')).toBe(graph.edges)
  })

  it('highlights edges connected to a hovered node and dims the rest', () => {
    const graph = buildTwoEdgeGraph()
    const gwKey = resourceKey(makeNode('Gateway', 'gw-1', 'default').ref)
    const result = applyEdgeHover(graph.edges, gwKey, 'light')

    // Both edges are connected to the hovered gateway, so both are highlighted.
    for (const edge of result) {
      expect(edge.zIndex).toBe(1)
      expect(edge.style?.opacity).toBeUndefined()
      expect(edge.labelStyle?.opacity).toBe(1)
    }
  })

  it('dims edges not connected to the hovered node', () => {
    const graph = buildTwoEdgeGraph()
    const routeAKey = resourceKey(makeNode('HTTPRoute', 'route-a', 'default').ref)
    const result = applyEdgeHover(graph.edges, routeAKey, 'light')

    const connected = result.find((e) => e.target === routeAKey)!
    const unconnected = result.find((e) => e.target !== routeAKey)!

    expect(connected.zIndex).toBe(1)
    expect(connected.labelStyle?.opacity).toBe(1)

    expect(unconnected.zIndex).toBeUndefined()
    expect(unconnected.style?.opacity).toBe(0.35)
    expect(unconnected.labelStyle?.opacity).toBe(0.35)
    expect(unconnected.labelBgStyle?.fillOpacity).toBe(0.35)
  })

  it('highlights a specific hovered edge by its own id', () => {
    const graph = buildTwoEdgeGraph()
    const targetEdgeId = graph.edges[0].id
    const result = applyEdgeHover(graph.edges, targetEdgeId, 'light')

    const highlighted = result.find((e) => e.id === targetEdgeId)!
    const other = result.find((e) => e.id !== targetEdgeId)!

    expect(highlighted.zIndex).toBe(1)
    expect(other.zIndex).toBeUndefined()
    expect(other.style?.opacity).toBe(0.35)
  })

  it('sets a distinct marker color for highlighted vs dimmed edges', () => {
    const graph = buildTwoEdgeGraph()
    const targetEdgeId = graph.edges[0].id
    const result = applyEdgeHover(graph.edges, targetEdgeId, 'light')

    const highlighted = result.find((e) => e.id === targetEdgeId)!
    const other = result.find((e) => e.id !== targetEdgeId)!

    const highlightedColor = (highlighted.markerEnd as {color?: string}).color
    const otherColor = (other.markerEnd as {color?: string}).color
    expect(highlightedColor).toBeTruthy()
    expect(otherColor).toBeTruthy()
    expect(highlightedColor).not.toBe(otherColor)
  })
})

// ---------------------------------------------------------------------------
// groupNodeCollapseThreshold
// ---------------------------------------------------------------------------

describe('groupNodeCollapseThreshold', () => {
  it('is a reasonable positive number', () => {
    expect(groupNodeCollapseThreshold).toBeGreaterThan(0)
    expect(groupNodeCollapseThreshold).toBe(8)
  })
})

// ---------------------------------------------------------------------------
// Helpers for extracting style info from flow nodes
// ---------------------------------------------------------------------------

function flowNodeBorderWidth(node: {style?: {border?: string | number}}): number {
  const border = String(node.style?.border ?? '')
  const match = border.match(/^([\d.]+)px/)
  return match ? parseFloat(match[1]) : 0
}

// ---------------------------------------------------------------------------
// namespaceGroupNodeId / isNamespaceGroupNodeId
// ---------------------------------------------------------------------------

describe('namespaceGroupNodeId', () => {
  it('prefixes namespace with namespace marker', () => {
    const id = namespaceGroupNodeId('default')
    expect(id).toContain('default')
    expect(isNamespaceGroupNodeId(id)).toBe(true)
  })
})

describe('isNamespaceGroupNodeId', () => {
  it('returns false for regular node ids', () => {
    expect(isNamespaceGroupNodeId('gateway.networking.k8s.io|Gateway|default|gw-1')).toBe(false)
  })

  it('returns true for namespace group ids', () => {
    expect(isNamespaceGroupNodeId(namespaceGroupNodeId('kube-system'))).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// applyNamespaceGrouping
// ---------------------------------------------------------------------------

describe('applyNamespaceGrouping', () => {
  it('returns nodes unchanged when only one namespace exists', () => {
    const nodes = [
      {id: 'node-1', position: {x: 0, y: 0}, data: {}},
      {id: 'node-2', position: {x: 100, y: 0}, data: {}},
    ]
    const snapshot = makePayload([
      makeNode('Gateway', 'gw-1', 'default'),
      makeNode('HTTPRoute', 'route-1', 'default'),
    ])

    const result = applyNamespaceGrouping(nodes, snapshot)
    expect(result).toBe(nodes) // same reference — no grouping applied
  })

  it('returns nodes unchanged when no namespaces exist', () => {
    const nodes = [{id: 'node-1', position: {x: 0, y: 0}, data: {}}]
    const snapshot = makePayload([makeNode('GatewayClass', 'gc-1')])

    const result = applyNamespaceGrouping(nodes, snapshot)
    expect(result).toBe(nodes)
  })

  it('creates namespace group parent nodes for multiple namespaces', () => {
    const gw = makeNode('Gateway', 'gw-1', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const snapshot = makePayload([gw, route])

    const flowNodes = [
      {id: resourceKey(gw.ref), position: {x: 50, y: 50}, data: {}, style: {width: 232}},
      {id: resourceKey(route.ref), position: {x: 50, y: 300}, data: {}, style: {width: 232}},
    ]

    const result = applyNamespaceGrouping(flowNodes, snapshot)

    // Should have 2 parent group nodes + 2 child nodes = 4 total
    expect(result).toHaveLength(4)

    const nsGroupNodes = result.filter((n) => isNamespaceGroupNodeId(n.id))
    expect(nsGroupNodes).toHaveLength(2)
    expect(nsGroupNodes[0].type).toBe('namespaceGroup')
    expect(nsGroupNodes[1].type).toBe('namespaceGroup')
  })

  it('assigns parentId to namespaced child nodes', () => {
    const gw = makeNode('Gateway', 'gw-1', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const snapshot = makePayload([gw, route])

    const flowNodes = [
      {id: resourceKey(gw.ref), position: {x: 50, y: 50}, data: {}, style: {width: 232}},
      {id: resourceKey(route.ref), position: {x: 50, y: 300}, data: {}, style: {width: 232}},
    ]

    const result = applyNamespaceGrouping(flowNodes, snapshot)

    const gwNode = result.find((n) => n.id === resourceKey(gw.ref))!
    expect(gwNode.parentId).toBe(namespaceGroupNodeId('ns-a'))

    const routeNode = result.find((n) => n.id === resourceKey(route.ref))!
    expect(routeNode.parentId).toBe(namespaceGroupNodeId('ns-b'))
  })

  it('converts child positions to relative coordinates', () => {
    const gw = makeNode('Gateway', 'gw-1', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const snapshot = makePayload([gw, route])

    const flowNodes = [
      {id: resourceKey(gw.ref), position: {x: 100, y: 200}, data: {}, style: {width: 232}},
      {id: resourceKey(route.ref), position: {x: 100, y: 500}, data: {}, style: {width: 232}},
    ]

    const result = applyNamespaceGrouping(flowNodes, snapshot)

    // Children should have relative positions (>= 0) within their parent.
    const gwNode = result.find((n) => n.id === resourceKey(gw.ref))!
    expect(gwNode.position.x).toBeGreaterThanOrEqual(0)
    expect(gwNode.position.y).toBeGreaterThanOrEqual(0)
  })

  it('places parent nodes before children in the array', () => {
    const gw = makeNode('Gateway', 'gw-1', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const snapshot = makePayload([gw, route])

    const flowNodes = [
      {id: resourceKey(gw.ref), position: {x: 0, y: 0}, data: {}, style: {width: 232}},
      {id: resourceKey(route.ref), position: {x: 0, y: 300}, data: {}, style: {width: 232}},
    ]

    const result = applyNamespaceGrouping(flowNodes, snapshot)

    // Find indices: all namespace group nodes should come before any child node.
    const groupIndices = result
      .map((n, i) => ({id: n.id, i}))
      .filter((x) => isNamespaceGroupNodeId(x.id))
      .map((x) => x.i)

    const childIndices = result
      .map((n, i) => ({id: n.id, i}))
      .filter((x) => x.id === resourceKey(gw.ref) || x.id === resourceKey(route.ref))
      .map((x) => x.i)

    const maxGroupIndex = Math.max(...groupIndices)
    const minChildIndex = Math.min(...childIndices)
    expect(maxGroupIndex).toBeLessThan(minChildIndex)
  })

  it('leaves cluster-scoped nodes ungrouped', () => {
    const gc = makeNode('GatewayClass', 'gc-1') // no namespace
    const gw = makeNode('Gateway', 'gw-1', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const snapshot = makePayload([gc, gw, route])

    const flowNodes = [
      {id: resourceKey(gc.ref), position: {x: 0, y: 0}, data: {}, style: {width: 232}},
      {id: resourceKey(gw.ref), position: {x: 0, y: 200}, data: {}, style: {width: 232}},
      {id: resourceKey(route.ref), position: {x: 0, y: 400}, data: {}, style: {width: 232}},
    ]

    const result = applyNamespaceGrouping(flowNodes, snapshot)

    const gcNode = result.find((n) => n.id === resourceKey(gc.ref))!
    expect(gcNode.parentId).toBeUndefined()
  })

  it('sizes parent node to enclose all children with padding', () => {
    const gw1 = makeNode('Gateway', 'gw-1', 'ns-a')
    const gw2 = makeNode('Gateway', 'gw-2', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const snapshot = makePayload([gw1, gw2, route])

    const flowNodes = [
      {id: resourceKey(gw1.ref), position: {x: 0, y: 0}, data: {}, style: {width: 232}},
      {id: resourceKey(gw2.ref), position: {x: 300, y: 0}, data: {}, style: {width: 232}},
      {id: resourceKey(route.ref), position: {x: 0, y: 400}, data: {}, style: {width: 232}},
    ]

    const result = applyNamespaceGrouping(flowNodes, snapshot)

    const nsAGroup = result.find((n) => n.id === namespaceGroupNodeId('ns-a'))!
    // Parent should be wider than a single node to enclose both gw-1 and gw-2.
    expect((nsAGroup.style?.width as number)).toBeGreaterThan(300)
    expect((nsAGroup.style?.height as number)).toBeGreaterThan(0)
  })

  it('resolves overlapping namespace groups', () => {
    // Two namespaces whose nodes produce overlapping bounding boxes.
    const gw = makeNode('Gateway', 'gw-1', 'ns-a')
    const route = makeNode('HTTPRoute', 'route-1', 'ns-b')
    const snapshot = makePayload([gw, route])

    // Both nodes at the same position — their group boxes will overlap.
    const flowNodes = [
      {id: resourceKey(gw.ref), position: {x: 50, y: 50}, data: {}, style: {width: 232}},
      {id: resourceKey(route.ref), position: {x: 50, y: 50}, data: {}, style: {width: 232}},
    ]

    const result = applyNamespaceGrouping(flowNodes, snapshot)

    const nsAGroup = result.find((n) => n.id === namespaceGroupNodeId('ns-a'))!
    const nsBGroup = result.find((n) => n.id === namespaceGroupNodeId('ns-b'))!

    // Groups should not overlap after resolution.
    const aRight = nsAGroup.position.x + (nsAGroup.style?.width as number)
    const bRight = nsBGroup.position.x + (nsBGroup.style?.width as number)
    const aBottom = nsAGroup.position.y + (nsAGroup.style?.height as number)
    const bBottom = nsBGroup.position.y + (nsBGroup.style?.height as number)

    const overlapX = Math.min(aRight, bRight) - Math.max(nsAGroup.position.x, nsBGroup.position.x)
    const overlapY = Math.min(aBottom, bBottom) - Math.max(nsAGroup.position.y, nsBGroup.position.y)

    const overlaps = overlapX > 0 && overlapY > 0
    expect(overlaps).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// resolveGroupOverlaps
// ---------------------------------------------------------------------------

describe('resolveGroupOverlaps', () => {
  function makeRect(ns: string, x: number, y: number, width: number, height: number): GroupRect {
    return {ns, x, y, width, height, children: []}
  }

  function rectsOverlap(a: GroupRect, b: GroupRect) {
    const overlapX = Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x)
    const overlapY = Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y)
    return overlapX > 0 && overlapY > 0
  }

  it('does nothing when there is only one group', () => {
    const rects = [makeRect('ns-a', 0, 0, 200, 150)]
    resolveGroupOverlaps(rects)
    expect(rects[0].x).toBe(0)
    expect(rects[0].y).toBe(0)
  })

  it('does nothing when groups do not overlap', () => {
    const rects = [
      makeRect('ns-a', 0, 0, 100, 100),
      makeRect('ns-b', 200, 0, 100, 100),
    ]
    resolveGroupOverlaps(rects)
    // No shift needed.
    expect(rects[0].x).toBe(0)
    expect(rects[1].x).toBe(200)
  })

  it('separates fully overlapping groups', () => {
    const rects = [
      makeRect('ns-a', 0, 0, 200, 150),
      makeRect('ns-b', 0, 0, 200, 150),
    ]
    resolveGroupOverlaps(rects)
    expect(rectsOverlap(rects[0], rects[1])).toBe(false)
  })

  it('separates partially overlapping groups', () => {
    const rects = [
      makeRect('ns-a', 0, 0, 200, 150),
      makeRect('ns-b', 100, 50, 200, 150),
    ]
    resolveGroupOverlaps(rects)
    expect(rectsOverlap(rects[0], rects[1])).toBe(false)
  })

  it('resolves three-way overlaps', () => {
    const rects = [
      makeRect('ns-a', 0, 0, 200, 150),
      makeRect('ns-b', 50, 30, 200, 150),
      makeRect('ns-c', 100, 60, 200, 150),
    ]
    resolveGroupOverlaps(rects)

    for (let i = 0; i < rects.length; i++) {
      for (let j = i + 1; j < rects.length; j++) {
        expect(rectsOverlap(rects[i], rects[j])).toBe(false)
      }
    }
  })

  it('shifts children along with the group', () => {
    const child = {id: 'child-1', position: {x: 50, y: 50}, data: {}} as Node
    const rects: GroupRect[] = [
      makeRect('ns-a', 0, 0, 200, 150),
      {ns: 'ns-b', x: 0, y: 0, width: 200, height: 150, children: [child]},
    ]
    const originalChildX = child.position.x
    const originalChildY = child.position.y

    resolveGroupOverlaps(rects)

    // The child should have shifted by the same amount as its group.
    const groupShiftedX = rects[1].x !== 0
    const groupShiftedY = rects[1].y !== 0

    if (groupShiftedX) {
      expect(child.position.x).not.toBe(originalChildX)
    }
    if (groupShiftedY) {
      expect(child.position.y).not.toBe(originalChildY)
    }
  })
})
