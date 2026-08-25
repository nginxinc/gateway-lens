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

import {MarkerType, type Edge, type Node} from '@xyflow/react'
import dagre from 'dagre'

import type {
  DashboardEdge,
  DashboardNode,
  DashboardPayload,
  DashboardResourceRef,
} from './types'
import {nodeColor, relationshipStyle} from './theme'

export const graphNodeWidth = 232
export const graphNodeHeight = 122
export const groupNodeCollapseThreshold = 8
export const referenceGrantSummaryAttribute = 'ReferenceGrant Summary'

const groupNodePrefix = '__group__'
const namespaceGroupPrefix = '__ns__'

export function groupNodeKey(kind: string) {
  return `${groupNodePrefix}${kind}`
}

export function namespaceGroupNodeId(namespace: string) {
  return `${namespaceGroupPrefix}${namespace}`
}

export function isNamespaceGroupNodeId(id: string) {
  return id.startsWith(namespaceGroupPrefix)
}

export function isGroupNodeKey(key: string) {
  return key.startsWith(groupNodePrefix)
}

export function resourceKey(ref: DashboardResourceRef) {
  return [ref.group, ref.kind, ref.namespace, ref.name].join('|')
}

export function edgeKey(edge: DashboardEdge) {
  return `${resourceKey(edge.from)}->${resourceKey(edge.to)}:${edge.type}:${edge.detail}`
}

export function formatResource(ref: DashboardResourceRef) {
  return ref.namespace ? `${ref.namespace}/${ref.name}` : ref.name
}

// errorConditionTypes are the condition types where False indicates an error
// warranting an error badge on the node.
const errorConditionTypes = new Set(['Accepted', 'Programmed', 'ResolvedRefs'])

function bareConditionType(conditionType: string) {
  // Condition types may be prefixed (e.g. "listener/http/Accepted"), so extract the suffix.
  if (!conditionType.includes('/')) return conditionType

  const segments = conditionType.split('/')
  return segments[segments.length - 1]
}

function isNegativeCondition(condition: {type: string; status: string}) {
  const bareType = bareConditionType(condition.type)
  return errorConditionTypes.has(bareType) && condition.status.toLowerCase() === 'false'
}

export function nodeHasNegativeCondition(node: DashboardNode) {
  return (node.conditions ?? []).some(isNegativeCondition)
}

export function applyFilters(
  snapshot: DashboardPayload,
  hiddenKinds: Set<string>,
  namespaceFilter: string,
  searchFilter: string,
): DashboardPayload {
  const search = searchFilter.toLowerCase().trim()

  const nodes = snapshot.nodes.filter((node) => {
    if (hiddenKinds.has(node.ref.kind)) return false
    if (namespaceFilter && node.ref.namespace !== namespaceFilter) return false
    if (search && !node.ref.name.toLowerCase().includes(search)) return false
    return true
  })

  const nodeKeys = new Set(nodes.map((n) => resourceKey(n.ref)))
  const edges = snapshot.edges.filter(
    (e) => nodeKeys.has(resourceKey(e.from)) && nodeKeys.has(resourceKey(e.to)),
  )
  const annotations = (snapshot.annotations ?? []).filter((a) =>
    nodeKeys.has(resourceKey(a.ref)),
  )

  return {...snapshot, nodes, edges, annotations}
}

export function applyCollapsing(
  snapshot: DashboardPayload,
  collapsedKinds: Set<string>,
): DashboardPayload {
  if (collapsedKinds.size === 0) return snapshot

  // Collect collapsed groups.
  const groupedNodes = new Map<string, DashboardNode[]>()
  const keptNodes: DashboardNode[] = []

  for (const node of snapshot.nodes) {
    if (collapsedKinds.has(node.ref.kind)) {
      const list = groupedNodes.get(node.ref.kind) ?? []
      list.push(node)
      groupedNodes.set(node.ref.kind, list)
    } else {
      keptNodes.push(node)
    }
  }

  // Create synthetic group nodes for each collapsed kind.
  for (const [kind, nodes] of groupedNodes) {
    keptNodes.push({
      ref: {group: nodes[0].ref.group, kind, namespace: '', name: groupNodeKey(kind)},
      attributes: {'__count': String(nodes.length)},
      conditions: [],
    })
  }

  // For collapsed kinds, redirect edges to/from any member to the group node.
  const collapsedNodeKeys = new Map<string, string>()
  for (const [kind, nodes] of groupedNodes) {
    const gKey = resourceKey({group: nodes[0].ref.group, kind, namespace: '', name: groupNodeKey(kind)})
    for (const node of nodes) {
      collapsedNodeKeys.set(resourceKey(node.ref), gKey)
    }
  }

  const seenEdges = new Set<string>()
  const edges: DashboardEdge[] = []
  for (const edge of snapshot.edges) {
    const fromKey = collapsedNodeKeys.get(resourceKey(edge.from))
    const toKey = collapsedNodeKeys.get(resourceKey(edge.to))

    const mappedEdge: DashboardEdge = {
      ...edge,
      from: fromKey ? refFromKey(fromKey) : edge.from,
      to: toKey ? refFromKey(toKey) : edge.to,
    }

    // Deduplicate edges between the same group endpoints.
    const dedupeKey = `${resourceKey(mappedEdge.from)}→${resourceKey(mappedEdge.to)}:${mappedEdge.detail}`
    if (seenEdges.has(dedupeKey)) continue
    seenEdges.add(dedupeKey)

    edges.push(mappedEdge)
  }

  const nodeKeys = new Set(keptNodes.map((n) => resourceKey(n.ref)))
  const validEdges = edges.filter(
    (e) => nodeKeys.has(resourceKey(e.from)) && nodeKeys.has(resourceKey(e.to)),
  )

  return {...snapshot, nodes: keptNodes, edges: validEdges, annotations: []}
}

function refFromKey(key: string): DashboardResourceRef {
  const [group, kind, namespace, name] = key.split('|')
  return {group, kind, namespace: namespace || undefined, name}
}

export type GroupNodeData = {
  kind: string
  count: number
  sourceBottomHandleCount: number
  sourceTopHandleCount: number
  targetBottomHandleCount: number
  targetTopHandleCount: number
}

export type NamespaceGroupNodeData = {
  namespace: string
}

export function buildGraph(snapshot: DashboardPayload, selectedResourceKey: string) {
  const graph = new dagre.graphlib.Graph()
  graph.setDefaultEdgeLabel(() => ({}))
  graph.setGraph({
    marginx: 40,
    marginy: 40,
    edgesep: 50,
    nodesep: 100,
    rankdir: 'TB',
    ranksep: 180,
  })

  snapshot.nodes.forEach((node) => {
    graph.setNode(resourceKey(node.ref), {
      height: graphNodeHeight,
      width: graphNodeWidth,
    })
  })

  snapshot.edges.forEach((edge, index) => {
    const layoutEndpoints = layoutEdgeEndpoints(edge)

    graph.setEdge(layoutEndpoints.source, layoutEndpoints.target, {
      id: `${edgeKey(edge)}:${index}`,
    })
  })

  dagre.layout(graph)

  // Pre-compute edge metadata and sort by opposite endpoint X-position so that
  // handle indices are assigned left-to-right, reducing edge crossings.
  const edgeMeta = snapshot.edges.map((edge, index) => {
    const visualEndpoints = visualEdgeEndpoints(edge)
    const sourcePosition = graph.node(visualEndpoints.source)
    const targetPosition = graph.node(visualEndpoints.target)
    const isUpwardEdge = targetPosition.y < sourcePosition.y
    const sourceSide = isUpwardEdge ? 'top' : 'bottom'
    const targetSide = isUpwardEdge ? 'bottom' : 'top'

    return {edge, index, visualEndpoints, sourcePosition, targetPosition, sourceSide, targetSide}
  })

  // Sort by the X-position of the opposite endpoint so handles are ordered spatially.
  const sortedForSourceHandles = [...edgeMeta].sort((a, b) => a.targetPosition.x - b.targetPosition.x)
  const sortedForTargetHandles = [...edgeMeta].sort((a, b) => a.sourcePosition.x - b.sourcePosition.x)

  const sourceTopHandleCount = new Map<string, number>()
  const sourceBottomHandleCount = new Map<string, number>()
  const targetTopHandleCount = new Map<string, number>()
  const targetBottomHandleCount = new Map<string, number>()

  // Assign source handles in order of target X-position.
  const sourceHandleMap = new Map<number, string>()
  for (const meta of sortedForSourceHandles) {
    const countMap = meta.sourceSide === 'top' ? sourceTopHandleCount : sourceBottomHandleCount
    const handleIndex = countMap.get(meta.visualEndpoints.source) ?? 0
    countMap.set(meta.visualEndpoints.source, handleIndex + 1)
    sourceHandleMap.set(meta.index, `source-${meta.sourceSide}-${handleIndex}`)
  }

  // Assign target handles in order of source X-position.
  const targetHandleMap = new Map<number, string>()
  for (const meta of sortedForTargetHandles) {
    const countMap = meta.targetSide === 'top' ? targetTopHandleCount : targetBottomHandleCount
    const handleIndex = countMap.get(meta.visualEndpoints.target) ?? 0
    countMap.set(meta.visualEndpoints.target, handleIndex + 1)
    targetHandleMap.set(meta.index, `target-${meta.targetSide}-${handleIndex}`)
  }

  const edges = edgeMeta.map((meta) => {
    const {edge, index, visualEndpoints, sourceSide, targetSide} = meta
    const edgeStyle = relationshipStyle()
    // Fall back to a deterministic handle name if, for some reason, an index was
    // not assigned above (should not happen since every edge is processed).
    const sourceHandle = sourceHandleMap.get(index) ?? `source-${sourceSide}-0`
    const targetHandle = targetHandleMap.get(index) ?? `target-${targetSide}-0`

    return {
      animated: edgeStyle.animated,
      id: `${edgeKey(edge)}:${index}`,
      label: edge.detail || edge.type,
      labelBgBorderRadius: 999,
      labelBgPadding: [8, 4],
      labelBgStyle: {fill: 'rgba(255, 250, 242, 0.92)', fillOpacity: 1},
      labelStyle: {fill: '#5f5b52', fontSize: 11, fontWeight: 600},
      markerEnd: {type: MarkerType.ArrowClosed, width: 18, height: 18},
      source: visualEndpoints.source,
      sourceHandle,
      style: edgeStyle.lineStyle,
      target: visualEndpoints.target,
      targetHandle,
      type: 'smoothstep',
    } satisfies Edge
  })

  const resourceNodes = snapshot.nodes.map((node) => {
    const key = resourceKey(node.ref)
    const position = graph.node(key)
    const tone = nodeColor(node.ref.kind)
    const isSelected = key === selectedResourceKey
    const isGroup = isGroupNodeKey(node.ref.name)

    if (isGroup) {
      // Count how many original nodes this group represents.
      // The count is encoded into the node name via the collapsing pass.
      const groupCount = Number(node.attributes?.['__count'] ?? 0)
      return {
        data: {
          kind: node.ref.kind,
          count: groupCount,
          sourceBottomHandleCount: sourceBottomHandleCount.get(key) ?? 0,
          sourceTopHandleCount: sourceTopHandleCount.get(key) ?? 0,
          targetBottomHandleCount: targetBottomHandleCount.get(key) ?? 0,
          targetTopHandleCount: targetTopHandleCount.get(key) ?? 0,
        } satisfies GroupNodeData,
        draggable: false,
        id: key,
        position: {
          x: position.x - graphNodeWidth / 2,
          y: position.y - graphNodeHeight / 2,
        },
        selectable: true,
        style: {
          background: tone.fill,
          border: `2px dashed ${tone.selectedStroke}`,
          borderRadius: 22,
          boxShadow: '0 12px 30px rgba(31, 43, 39, 0.1)',
          padding: 0,
          width: graphNodeWidth,
        },
        type: 'group',
      } satisfies Node
    }

    const hasNegativeCondition = nodeHasNegativeCondition(node)
    const borderColor = hasNegativeCondition
      ? isSelected
        ? 'rgba(171, 32, 21, 1)'
        : 'rgba(171, 32, 21, 0.72)'
      : isSelected
        ? tone.selectedStroke
        : tone.stroke
    const borderWidth = hasNegativeCondition ? 2.6 : 1
    const boxShadow = hasNegativeCondition
      ? isSelected
        ? '0 0 0 4px rgba(193, 49, 38, 0.2), 0 22px 60px rgba(31, 43, 39, 0.18)'
        : '0 0 0 3px rgba(193, 49, 38, 0.16), 0 12px 30px rgba(31, 43, 39, 0.1)'
      : isSelected
        ? '0 22px 60px rgba(31, 43, 39, 0.18)'
        : '0 12px 30px rgba(31, 43, 39, 0.1)'

    return {
      data: {
        displayName: formatResource(node.ref),
        detailText: referenceGrantNodeSummary(node),
        hasNegativeCondition,
        kind: node.ref.kind,
        sourceBottomHandleCount: sourceBottomHandleCount.get(key) ?? 0,
        sourceTopHandleCount: sourceTopHandleCount.get(key) ?? 0,
        targetBottomHandleCount: targetBottomHandleCount.get(key) ?? 0,
        targetTopHandleCount: targetTopHandleCount.get(key) ?? 0,
      },
      draggable: false,
      id: key,
      position: {
        x: position.x - graphNodeWidth / 2,
        y: position.y - graphNodeHeight / 2,
      },
      selectable: true,
      style: {
        background: isSelected ? tone.selectedFill : tone.fill,
        border: `${borderWidth}px solid ${borderColor}`,
        borderRadius: 22,
        boxShadow,
        padding: 0,
        width: graphNodeWidth,
      },
      type: 'resource',
    } satisfies Node
  })

  const nodes = applyNamespaceGrouping(resourceNodes, snapshot)

  return {edges, nodes}
}

// Padding inside namespace group boxes around child nodes.
const namespacePadding = {top: 40, right: 24, bottom: 24, left: 24}

// applyNamespaceGrouping creates parent container nodes for each namespace
// and re-parents namespaced resource nodes inside them. Cluster-scoped nodes
// (no namespace) remain ungrouped. When only one namespace is present, grouping
// is skipped to avoid visual noise.
export function applyNamespaceGrouping(
  resourceNodes: Node[],
  snapshot: DashboardPayload,
): Node[] {
  // Collect distinct namespaces from the original snapshot data.
  const namespaces = new Set<string>()
  for (const node of snapshot.nodes) {
    if (node.ref.namespace) {
      namespaces.add(node.ref.namespace)
    }
  }

  // Skip grouping when there's 0 or 1 namespace — no visual benefit.
  if (namespaces.size <= 1) {
    return resourceNodes
  }

  // Build a lookup from node id to its original snapshot data for namespace info.
  const nodeNamespaceMap = new Map<string, string>()
  for (const node of snapshot.nodes) {
    if (node.ref.namespace) {
      nodeNamespaceMap.set(resourceKey(node.ref), node.ref.namespace)
    }
  }

  // Group resource nodes by namespace.
  const grouped = new Map<string, Node[]>()
  const ungrouped: Node[] = []

  for (const node of resourceNodes) {
    const ns = nodeNamespaceMap.get(node.id)
    if (ns) {
      const list = grouped.get(ns) ?? []
      list.push(node)
      grouped.set(ns, list)
    } else {
      ungrouped.push(node)
    }
  }

  // Compute bounding boxes for each namespace group.
  const groupRects: GroupRect[] = []

  for (const [ns, children] of grouped) {
    if (children.length === 0) continue

    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity

    for (const child of children) {
      const childWidth = (child.style?.width as number) ?? graphNodeWidth
      minX = Math.min(minX, child.position.x)
      minY = Math.min(minY, child.position.y)
      maxX = Math.max(maxX, child.position.x + childWidth)
      maxY = Math.max(maxY, child.position.y + graphNodeHeight)
    }

    groupRects.push({
      ns,
      x: minX - namespacePadding.left,
      y: minY - namespacePadding.top,
      width: maxX - minX + namespacePadding.left + namespacePadding.right,
      height: maxY - minY + namespacePadding.top + namespacePadding.bottom,
      children,
    })
  }

  // Resolve overlaps between namespace group boxes.
  resolveGroupOverlaps(groupRects)

  // Create parent nodes and reparent children using resolved positions.
  const parentNodes: Node[] = []

  for (const rect of groupRects) {
    const parentId = namespaceGroupNodeId(rect.ns)

    parentNodes.push({
      data: {namespace: rect.ns} satisfies NamespaceGroupNodeData,
      draggable: false,
      id: parentId,
      position: {x: rect.x, y: rect.y},
      selectable: false,
      style: {
        height: rect.height,
        width: rect.width,
      },
      type: 'namespaceGroup',
    } satisfies Node)

    // Convert children to relative positions within the parent.
    for (const child of rect.children) {
      child.parentId = parentId
      child.position = {
        x: child.position.x - rect.x,
        y: child.position.y - rect.y,
      }
    }
  }

  // React Flow requires parents before children in the array.
  const childNodes: Node[] = []
  for (const nodes of grouped.values()) {
    childNodes.push(...nodes)
  }

  return [...parentNodes, ...ungrouped, ...childNodes]
}

// Gap between namespace group boxes after overlap resolution.
const namespaceGroupGap = 24

export interface GroupRect {
  ns: string
  x: number
  y: number
  width: number
  height: number
  children: Node[]
}

// resolveGroupOverlaps pushes namespace group rectangles apart so they never
// overlap. It iterates until no overlaps remain, shifting the later rectangle
// along the axis of least penetration.
export function resolveGroupOverlaps(rects: GroupRect[]) {
  if (rects.length < 2) return

  // Sort by Y then X so the shift direction is deterministic and stable.
  rects.sort((a, b) => a.y - b.y || a.x - b.x)

  const maxIterations = rects.length * rects.length
  for (let iteration = 0; iteration < maxIterations; iteration++) {
    let anyOverlap = false

    for (let i = 0; i < rects.length; i++) {
      for (let j = i + 1; j < rects.length; j++) {
        const a = rects[i]
        const b = rects[j]

        const overlapX = Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x)
        const overlapY = Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y)

        if (overlapX <= 0 || overlapY <= 0) continue

        anyOverlap = true

        // Shift along the axis of least penetration.
        if (overlapX < overlapY) {
          // Push horizontally.
          const shift = overlapX + namespaceGroupGap
          if (a.x <= b.x) {
            shiftGroup(b, shift, 0)
          } else {
            shiftGroup(a, shift, 0)
          }
        } else {
          // Push vertically.
          const shift = overlapY + namespaceGroupGap
          if (a.y <= b.y) {
            shiftGroup(b, 0, shift)
          } else {
            shiftGroup(a, 0, shift)
          }
        }
      }
    }

    if (!anyOverlap) break
  }
}

function shiftGroup(rect: GroupRect, dx: number, dy: number) {
  rect.x += dx
  rect.y += dy

  // Shift children by the same amount so relative positions stay correct
  // when reparenting later subtracts the new rect origin.
  for (const child of rect.children) {
    child.position = {
      x: child.position.x + dx,
      y: child.position.y + dy,
    }
  }
}

export function visibleGraphSnapshot(snapshot: DashboardPayload): DashboardPayload {
  const referenceGrantFromNamespaces = new Map<string, Set<string>>()
  const referenceGrantTargets = new Map<string, Set<string>>()

  snapshot.edges.forEach((edge) => {
    if (edge.detail === 'referenceGrantFrom' && edge.to.kind === 'ReferenceGrant') {
      const fromNamespace = edge.from.namespace || 'all namespaces'
      addToSetMap(referenceGrantFromNamespaces, resourceKey(edge.to), fromNamespace)
    }

    if (edge.detail === 'referenceGrantTo' && edge.from.kind === 'ReferenceGrant') {
      addToSetMap(referenceGrantTargets, resourceKey(edge.from), formatResource(edge.to))
    }
  })

  const nodes = snapshot.nodes
    .filter((node) => !isSyntheticWildcardRoute(node.ref))
    .map((node) => {
      if (node.ref.kind !== 'ReferenceGrant') {
        return node
      }

      const nodeKey = resourceKey(node.ref)
      const fromNamespaces = Array.from(referenceGrantFromNamespaces.get(nodeKey) ?? [])
      const targets = Array.from(referenceGrantTargets.get(nodeKey) ?? [])
      const summary = describeReferenceGrant(fromNamespaces, targets)
      if (!summary) {
        return node
      }

      return {
        ...node,
        attributes: {
          ...node.attributes,
          [referenceGrantSummaryAttribute]: summary,
        },
      }
    })

  const nodeKeys = new Set(nodes.map((node) => resourceKey(node.ref)))
  const edges = snapshot.edges.filter((edge) => {
    if (isSyntheticWildcardRoute(edge.from) || isSyntheticWildcardRoute(edge.to)) {
      return false
    }

    return nodeKeys.has(resourceKey(edge.from)) && nodeKeys.has(resourceKey(edge.to))
  })
  const annotations = (snapshot.annotations ?? []).filter((annotation) => {
    return nodeKeys.has(resourceKey(annotation.ref))
  })

  return {
    ...snapshot,
    annotations,
    edges,
    nodes,
  }
}

export function resolveSelectedKey(snapshot: DashboardPayload, currentSelectedKey: string) {
  if (!snapshot.nodes.length) {
    return ''
  }

  if (snapshot.nodes.some((node) => resourceKey(node.ref) === currentSelectedKey)) {
    return currentSelectedKey
  }

  return resourceKey(snapshot.nodes[0].ref)
}

function visualEdgeEndpoints(edge: DashboardEdge) {
  return {
    source: resourceKey(edge.from),
    target: resourceKey(edge.to),
  }
}

function layoutEdgeEndpoints(edge: DashboardEdge) {
  if (shouldReverseForLayout(edge)) {
    return {
      source: resourceKey(edge.to),
      target: resourceKey(edge.from),
    }
  }

  return visualEdgeEndpoints(edge)
}

function shouldReverseForLayout(edge: DashboardEdge) {
  return edge.detail === 'gatewayClass' || edge.detail === 'parentRef'
}

function isSyntheticWildcardRoute(ref: DashboardResourceRef) {
  return ref.name === '*' && ref.kind.endsWith('Route')
}

function referenceGrantNodeSummary(node: DashboardNode) {
  if (node.ref.kind !== 'ReferenceGrant') {
    return undefined
  }

  return node.attributes?.[referenceGrantSummaryAttribute]
}

function addToSetMap(collection: Map<string, Set<string>>, key: string, value: string) {
  const existingValues = collection.get(key)
  if (existingValues) {
    existingValues.add(value)
    return
  }

  collection.set(key, new Set([value]))
}

function describeReferenceGrant(fromNamespaces: string[], targets: string[]) {
  if (!fromNamespaces.length || !targets.length) {
    return ''
  }

  const namespacedFrom = fromNamespaces.map((namespace) => {
    if (namespace === 'all namespaces') {
      return namespace
    }

    return `${namespace} namespace`
  })
  const fromLabel =
    namespacedFrom.length === 1
      ? `HTTPRoutes in ${namespacedFrom[0]}`
      : `HTTPRoutes in ${namespacedFrom.join(', ')}`
  const targetLabel = targets.length === 1 ? targets[0] : targets.join(', ')

  return `${fromLabel} can reference ${targetLabel}`
}

export function buildSelectedResourceYAML(node: DashboardNode) {
  return node.manifest || '# Manifest unavailable for this resource'
}

export function apiURL(pathname: string) {
  const apiBaseURL = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? ''
  return `${apiBaseURL}${pathname}`
}
