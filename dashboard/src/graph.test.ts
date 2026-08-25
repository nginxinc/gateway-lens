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

import {describe, expect, it} from 'vitest'

import type {DashboardEdge, DashboardNode, DashboardPayload, DashboardResourceRef} from './types'
import type {Node} from '@xyflow/react'

import type {GroupRect} from './graph'
import {
  apiURL,
  applyCollapsing,
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

  it('falls back to first node if current key is gone', () => {
    const node = makeNode('Gateway', 'gw-1')
    const result = resolveSelectedKey(makePayload([node]), 'nonexistent')
    expect(result).toBe(resourceKey(node.ref))
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
  it('appends pathname to base', () => {
    // import.meta.env.VITE_API_BASE_URL is undefined in test, so base is ""
    expect(apiURL('/api/graph')).toBe('/api/graph')
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
