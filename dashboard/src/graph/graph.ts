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

import type {ColorScheme} from '../state/colorScheme'
import type {
  DashboardEdge,
  DashboardNode,
  DashboardPayload,
  DashboardResourceRef,
} from '../types'
import {dimmedRelationshipMarkerColor, highlightRelationshipStroke, nodeColor, relationshipStyle} from './theme'

export const graphNodeWidth = 232
export const graphNodeHeight = 122
export const groupNodeCollapseThreshold = 8
export const referenceGrantSummaryAttribute = 'ReferenceGrant Summary'
export const serviceReadyEndpointsAttribute = 'readyEndpoints'
export const serviceTotalEndpointsAttribute = 'totalEndpoints'

export const rankRowMaxColumns = 4

const reflowCrossGap = 40
const reflowRankGap = 30

// reflowWideRanks re-arranges ranks that have more than rankRowMaxColumns nodes
// into a multi-row grid. It mutates the dagre graph node positions in-place and
// shifts downstream ranks to accommodate the extra rows.
export function reflowWideRanks(
  graph: dagre.graphlib.Graph,
  orientation: GraphOrientation,
  maxColumns: number = rankRowMaxColumns,
) {
  const rKey = rankAxis(orientation)
  const cKey = crossAxis(orientation)
  const nodeWidth = graphNodeWidth
  const nodeHeight = graphNodeHeight

  // Group nodes by their rank-axis coordinate, using a tolerance to handle
  // floating-point variations from dagre. Nodes within rankGroupTolerance
  // pixels of each other on the rank axis are considered the same rank.
  const rankGroupTolerance = 1

  const nodeEntries: Array<{id: string; rankVal: number}> = []
  for (const id of graph.nodes()) {
    const pos = graph.node(id)
    if (!pos) continue
    nodeEntries.push({id, rankVal: pos[rKey]})
  }
  nodeEntries.sort((a, b) => a.rankVal - b.rankVal)

  const sortedRanks: Array<[number, string[]]> = []
  for (const entry of nodeEntries) {
    const last = sortedRanks[sortedRanks.length - 1]
    if (last && Math.abs(entry.rankVal - last[0]) <= rankGroupTolerance) {
      last[1].push(entry.id)
    } else {
      sortedRanks.push([entry.rankVal, [entry.id]])
    }
  }

  let cumulativeShift = 0

  for (const [, nodeIds] of sortedRanks) {
    if (cumulativeShift !== 0) {
      for (const id of nodeIds) {
        const pos = graph.node(id)
        pos[rKey] = pos[rKey] + cumulativeShift
      }
    }

    if (nodeIds.length <= maxColumns) continue

    nodeIds.sort((a, b) => graph.node(a)[cKey] - graph.node(b)[cKey])

    const cols = maxColumns
    const rows = Math.ceil(nodeIds.length / cols)

    const cellCross = (rKey === 'y' ? nodeWidth : nodeHeight) + reflowCrossGap
    const cellRank = (rKey === 'y' ? nodeHeight : nodeWidth) + reflowRankGap

    const positions = nodeIds.map((id) => graph.node(id))
    const minCross = Math.min(...positions.map((p) => p[cKey]))
    const maxCross = Math.max(...positions.map((p) => p[cKey]))
    const crossCenter = (minCross + maxCross) / 2

    const gridCrossWidth = cols * cellCross - reflowCrossGap
    const gridCrossStart = crossCenter - gridCrossWidth / 2

    const rankCenter = positions[0][rKey]

    for (let i = 0; i < nodeIds.length; i++) {
      const col = i % cols
      const row = Math.floor(i / cols)
      const pos = graph.node(nodeIds[i])
      pos[cKey] = gridCrossStart + col * cellCross
      pos[rKey] = rankCenter + row * cellRank
    }

    const extraRankSpace = (rows - 1) * cellRank
    cumulativeShift += extraRankSpace
  }
}

const selectionRingColor = 'rgba(37, 99, 235, 0.9)'
const selectionGlowColor = 'rgba(37, 99, 235, 0.55)'

const errorGlowColors = {
  light: {
    border: 'rgba(171, 32, 21, 1)',
    borderDim: 'rgba(171, 32, 21, 0.72)',
    ringSelected: 'rgba(193, 49, 38, 0.2)',
    ringUnselected: 'rgba(193, 49, 38, 0.16)',
  },
  dark: {
    border: 'rgba(255, 120, 105, 1)',
    borderDim: 'rgba(255, 120, 105, 0.75)',
    ringSelected: 'rgba(255, 120, 105, 0.28)',
    ringUnselected: 'rgba(255, 120, 105, 0.22)',
  },
} satisfies Record<ColorScheme, {border: string; borderDim: string; ringSelected: string; ringUnselected: string}>

const shadowColors = {
  light: {ambient: 'rgba(31, 43, 39, 0.1)', elevated: 'rgba(31, 43, 39, 0.18)'},
  dark: {ambient: 'rgba(0, 0, 0, 0.45)', elevated: 'rgba(0, 0, 0, 0.6)'},
} satisfies Record<ColorScheme, {ambient: string; elevated: string}>

const groupNodePrefix = '__group__'
const namespaceGroupPrefix = '__ns__'

export function groupNodeKey(kind: string, namespace?: string) {
  if (namespace) {
    return `${groupNodePrefix}${kind}__${namespace}`
  }
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

function isErrorDiagnostic(diagnostic: {severity: string}) {
  return diagnostic.severity.toLowerCase() === 'error'
}

export function nodeHasNegativeCondition(node: DashboardNode) {
  return (node.conditions ?? []).some(isNegativeCondition) || (node.diagnostics ?? []).some(isErrorDiagnostic)
}

export function matchesSearch(ref: DashboardResourceRef, searchFilter: string): boolean {
  const trimmed = searchFilter.trim()
  if (!trimmed) return true

  const slashIndex = trimmed.indexOf('/')
  if (slashIndex !== -1) {
    const namespaceTerm = trimmed.slice(0, slashIndex).trim().toLowerCase()
    const nameTerm = trimmed.slice(slashIndex + 1).trim().toLowerCase()
    const namespaceMatches = namespaceTerm === '' || (ref.namespace ?? '').toLowerCase().includes(namespaceTerm)
    const nameMatches = nameTerm === '' || ref.name.toLowerCase().includes(nameTerm)
    return namespaceMatches && nameMatches
  }

  const term = trimmed.toLowerCase()
  return ref.name.toLowerCase().includes(term) || (ref.namespace ?? '').toLowerCase().includes(term)
}

export function applyFilters(
  snapshot: DashboardPayload,
  hiddenKinds: Set<string>,
  namespaceFilter: string,
  searchFilter: string,
): DashboardPayload {
  const nodes = snapshot.nodes.filter((node) => {
    if (hiddenKinds.has(node.ref.kind)) return false
    if (namespaceFilter && node.ref.namespace !== namespaceFilter) return false
    if (!matchesSearch(node.ref, searchFilter)) return false
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

export function collapseGroupKey(kind: string, namespace: string) {
  return `${kind}|${namespace}`
}

export function applyCollapsing(
  snapshot: DashboardPayload,
  collapsedKinds: Set<string>,
  expandedGroups: Set<string> = new Set(),
): DashboardPayload {
  if (collapsedKinds.size === 0) return snapshot

  // Collect collapsed groups keyed by kind+namespace.
  // A node is collapsed if its kind is in collapsedKinds AND its
  // kind|namespace pair is NOT in expandedGroups.
  const groupedNodes = new Map<string, DashboardNode[]>()
  const keptNodes: DashboardNode[] = []

  for (const node of snapshot.nodes) {
    const ns = node.ref.namespace ?? ''
    if (collapsedKinds.has(node.ref.kind) && !expandedGroups.has(collapseGroupKey(node.ref.kind, ns))) {
      const mapKey = `${node.ref.kind}|${ns}`
      const list = groupedNodes.get(mapKey) ?? []
      list.push(node)
      groupedNodes.set(mapKey, list)
    } else {
      keptNodes.push(node)
    }
  }

  // Create synthetic group nodes for each collapsed kind+namespace pair.
  for (const [, nodes] of groupedNodes) {
    const kind = nodes[0].ref.kind
    const ns = nodes[0].ref.namespace ?? ''
    keptNodes.push({
      ref: {group: nodes[0].ref.group, kind, namespace: ns || undefined, name: groupNodeKey(kind, ns || undefined)},
      attributes: {'__count': String(nodes.length)},
      conditions: [],
    })
  }

  // For collapsed kinds, redirect edges to/from any member to the group node.
  const collapsedNodeKeys = new Map<string, string>()
  for (const [, nodes] of groupedNodes) {
    const kind = nodes[0].ref.kind
    const ns = nodes[0].ref.namespace ?? ''
    const gKey = resourceKey({group: nodes[0].ref.group, kind, namespace: ns || undefined, name: groupNodeKey(kind, ns || undefined)})
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

// GraphOrientation selects which direction dagre lays the topology out in.
// 'TB' (top-to-bottom, the original/default) reads request flow downward;
// 'LR' (left-to-right) reads it left-to-right.
export type GraphOrientation = 'TB' | 'LR'

export const defaultGraphOrientation: GraphOrientation = 'TB'

type HandleSide = 'top' | 'bottom' | 'left' | 'right'

export type GroupNodeData = {
  kind: string
  namespace?: string
  count: number
  sourceBottomHandleCount: number
  sourceTopHandleCount: number
  sourceLeftHandleCount: number
  sourceRightHandleCount: number
  targetBottomHandleCount: number
  targetTopHandleCount: number
  targetLeftHandleCount: number
  targetRightHandleCount: number
}

export type NamespaceGroupNodeData = {
  namespace: string
}

// rankAxis is the coordinate axis dagre ranks nodes along for a given
// orientation ('y' for top-to-bottom, 'x' for left-to-right). crossAxis is
// the perpendicular axis used to order handles spatially within a rank.
function rankAxis(orientation: GraphOrientation): 'x' | 'y' {
  return orientation === 'LR' ? 'x' : 'y'
}

function crossAxis(orientation: GraphOrientation): 'x' | 'y' {
  return orientation === 'LR' ? 'y' : 'x'
}

// forwardSide/backwardSide are the node edges edges exit/enter from when
// travelling in the "normal" (forward) vs. reversed direction of the rank axis.
function forwardSide(orientation: GraphOrientation): HandleSide {
  return orientation === 'LR' ? 'right' : 'bottom'
}

function backwardSide(orientation: GraphOrientation): HandleSide {
  return orientation === 'LR' ? 'left' : 'top'
}

export function buildGraph(
  snapshot: DashboardPayload,
  selectedResourceKey: string,
  colorScheme: ColorScheme = 'light',
  orientation: GraphOrientation = defaultGraphOrientation,
) {
  const graph = new dagre.graphlib.Graph()
  graph.setDefaultEdgeLabel(() => ({}))
  graph.setGraph({
    marginx: 40,
    marginy: 40,
    edgesep: 70,
    nodesep: 130,
    rankdir: orientation,
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
  reflowWideRanks(graph, orientation)

  const rankKey = rankAxis(orientation)
  const crossKey = crossAxis(orientation)
  const forward = forwardSide(orientation)
  const backward = backwardSide(orientation)

  // Pre-compute edge metadata and sort by opposite endpoint's cross-axis
  // position so that handle indices are assigned spatially, reducing edge
  // crossings (X-position for TB layouts, Y-position for LR layouts).
  const edgeMeta = snapshot.edges.map((edge, index) => {
    const visualEndpoints = visualEdgeEndpoints(edge)
    const sourcePosition = graph.node(visualEndpoints.source)
    const targetPosition = graph.node(visualEndpoints.target)
    const isBackwardEdge = targetPosition[rankKey] < sourcePosition[rankKey]
    const sourceSide = isBackwardEdge ? backward : forward
    const targetSide = isBackwardEdge ? forward : backward

    return {edge, index, visualEndpoints, sourcePosition, targetPosition, sourceSide, targetSide}
  })

  // Sort by the cross-axis position of the opposite endpoint so handles are ordered spatially.
  const sortedForSourceHandles = [...edgeMeta].sort(
    (a, b) => a.targetPosition[crossKey] - b.targetPosition[crossKey],
  )
  const sortedForTargetHandles = [...edgeMeta].sort(
    (a, b) => a.sourcePosition[crossKey] - b.sourcePosition[crossKey],
  )

  const sourceHandleCounts: Record<HandleSide, Map<string, number>> = {
    top: new Map(),
    bottom: new Map(),
    left: new Map(),
    right: new Map(),
  }
  const targetHandleCounts: Record<HandleSide, Map<string, number>> = {
    top: new Map(),
    bottom: new Map(),
    left: new Map(),
    right: new Map(),
  }

  // Assign source handles in order of the opposite endpoint's cross-axis position.
  const sourceHandleMap = new Map<number, string>()
  for (const meta of sortedForSourceHandles) {
    const countMap = sourceHandleCounts[meta.sourceSide]
    const handleIndex = countMap.get(meta.visualEndpoints.source) ?? 0
    countMap.set(meta.visualEndpoints.source, handleIndex + 1)
    sourceHandleMap.set(meta.index, `source-${meta.sourceSide}-${handleIndex}`)
  }

  // Assign target handles in order of the opposite endpoint's cross-axis position.
  const targetHandleMap = new Map<number, string>()
  for (const meta of sortedForTargetHandles) {
    const countMap = targetHandleCounts[meta.targetSide]
    const handleIndex = countMap.get(meta.visualEndpoints.target) ?? 0
    countMap.set(meta.visualEndpoints.target, handleIndex + 1)
    targetHandleMap.set(meta.index, `target-${meta.targetSide}-${handleIndex}`)
  }

  const edges = edgeMeta.map((meta) => {
    const {edge, index, visualEndpoints, sourceSide, targetSide} = meta
    const edgeStyle = relationshipStyle(colorScheme)
    // Fall back to a deterministic handle name if, for some reason, an index was
    // not assigned above (should not happen since every edge is processed).
    const sourceHandle = sourceHandleMap.get(index) ?? `source-${sourceSide}-0`
    const targetHandle = targetHandleMap.get(index) ?? `target-${targetSide}-0`

    return {
      animated: edgeStyle.animated,
      id: `${edgeKey(edge)}:${index}`,
      markerEnd: {type: MarkerType.ArrowClosed, width: 18, height: 18},
      source: visualEndpoints.source,
      sourceHandle,
      style: edgeStyle.lineStyle,
      target: visualEndpoints.target,
      targetHandle,
      type: 'smoothstep',
    } satisfies Edge
  })

  const errorGlow = errorGlowColors[colorScheme]
  const shadow = shadowColors[colorScheme]

  const resourceNodes = snapshot.nodes.map((node) => {
    const key = resourceKey(node.ref)
    const position = graph.node(key)
    const tone = nodeColor(node.ref.kind, colorScheme)
    const isSelected = key === selectedResourceKey
    const isGroup = isGroupNodeKey(node.ref.name)

    if (isGroup) {
      // Count how many original nodes this group represents.
      // The count is encoded into the node name via the collapsing pass.
      const groupCount = Number(node.attributes?.['__count'] ?? 0)
      return {
        data: {
          kind: node.ref.kind,
          namespace: node.ref.namespace,
          count: groupCount,
          sourceBottomHandleCount: sourceHandleCounts.bottom.get(key) ?? 0,
          sourceTopHandleCount: sourceHandleCounts.top.get(key) ?? 0,
          sourceLeftHandleCount: sourceHandleCounts.left.get(key) ?? 0,
          sourceRightHandleCount: sourceHandleCounts.right.get(key) ?? 0,
          targetBottomHandleCount: targetHandleCounts.bottom.get(key) ?? 0,
          targetTopHandleCount: targetHandleCounts.top.get(key) ?? 0,
          targetLeftHandleCount: targetHandleCounts.left.get(key) ?? 0,
          targetRightHandleCount: targetHandleCounts.right.get(key) ?? 0,
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
          boxShadow: `0 12px 30px ${shadow.ambient}`,
          padding: 0,
          width: graphNodeWidth,
        },
        type: 'group',
      } satisfies Node
    }

    const hasNegativeCondition = nodeHasNegativeCondition(node)
    const borderColor = hasNegativeCondition
      ? isSelected
        ? errorGlow.border
        : errorGlow.borderDim
      : isSelected
        ? tone.selectedStroke
        : tone.stroke
    const borderWidth = hasNegativeCondition || isSelected ? 2.6 : 1
    const selectionRing = isSelected
      ? `0 0 0 4px ${selectionRingColor}, 0 0 24px 2px ${selectionGlowColor}`
      : ''
    const conditionGlow = hasNegativeCondition
      ? isSelected
        ? `0 0 0 4px ${errorGlow.ringSelected}, 0 22px 60px ${shadow.elevated}`
        : `0 0 0 3px ${errorGlow.ringUnselected}, 0 12px 30px ${shadow.ambient}`
      : isSelected
        ? `0 22px 60px ${shadow.elevated}`
        : `0 12px 30px ${shadow.ambient}`
    const boxShadow = selectionRing ? `${selectionRing}, ${conditionGlow}` : conditionGlow

    return {
      data: {
        displayName: formatResource(node.ref),
        detailText: referenceGrantNodeSummary(node),
        hasNegativeCondition,
        kind: node.ref.kind,
        readinessBadge: serviceReadinessBadge(node),
        sourceBottomHandleCount: sourceHandleCounts.bottom.get(key) ?? 0,
        sourceTopHandleCount: sourceHandleCounts.top.get(key) ?? 0,
        sourceLeftHandleCount: sourceHandleCounts.left.get(key) ?? 0,
        sourceRightHandleCount: sourceHandleCounts.right.get(key) ?? 0,
        targetBottomHandleCount: targetHandleCounts.bottom.get(key) ?? 0,
        targetTopHandleCount: targetHandleCounts.top.get(key) ?? 0,
        targetLeftHandleCount: targetHandleCounts.left.get(key) ?? 0,
        targetRightHandleCount: targetHandleCounts.right.get(key) ?? 0,
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

export function applyEdgeHover(edges: Edge[], hoveredKey: string, colorScheme: ColorScheme = 'light'): Edge[] {
  if (!hoveredKey) return edges

  const highlightStroke = highlightRelationshipStroke(colorScheme)
  const dimmedMarkerColor = dimmedRelationshipMarkerColor(colorScheme)

  return edges.map((edge) => {
    const isHighlighted = edge.id === hoveredKey || edge.source === hoveredKey || edge.target === hoveredKey

    const baseLineStyle = edge.style ?? {}
    const style = isHighlighted
      ? {
          ...baseLineStyle,
          stroke: highlightStroke,
          strokeWidth: (typeof baseLineStyle.strokeWidth === 'number' ? baseLineStyle.strokeWidth : 1.8) + 0.8,
        }
      : {...baseLineStyle, opacity: 0.35}

    const markerEnd =
      typeof edge.markerEnd === 'object' && edge.markerEnd
        ? {...edge.markerEnd, color: isHighlighted ? highlightStroke : dimmedMarkerColor}
        : edge.markerEnd

    return {
      ...edge,
      markerEnd,
      style,
      zIndex: isHighlighted ? 1 : undefined,
    } satisfies Edge
  })
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

  // Push cluster-scoped (ungrouped) nodes out of namespace boxes so they
  // never render inside a namespace container (e.g. GatewayClass).
  resolveUngroupedOverlaps(ungrouped, groupRects)

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

// resolveUngroupedOverlaps pushes cluster-scoped nodes (like GatewayClass)
// out of namespace group boxes so they never visually appear inside one.
// It iterates until no overlaps remain, because pushing a node out of one box
// can land it inside another.
export function resolveUngroupedOverlaps(ungrouped: Node[], rects: GroupRect[]) {
  if (ungrouped.length === 0 || rects.length === 0) return

  const maxIterations = ungrouped.length * rects.length * 2

  for (const node of ungrouped) {
    const nodeWidth = (node.style?.width as number) ?? graphNodeWidth
    const nodeHeight = graphNodeHeight

    for (let iter = 0; iter < maxIterations; iter++) {
      let pushed = false

      for (const rect of rects) {
        const overlapX =
          Math.min(node.position.x + nodeWidth, rect.x + rect.width) -
          Math.max(node.position.x, rect.x)
        const overlapY =
          Math.min(node.position.y + nodeHeight, rect.y + rect.height) -
          Math.max(node.position.y, rect.y)

        if (overlapX <= 0 || overlapY <= 0) continue

        // Push along the axis of least penetration.
        if (overlapY <= overlapX) {
          // Push above or below depending on which is closer.
          const pushAbove = node.position.y < rect.y + rect.height / 2
          node.position = {
            x: node.position.x,
            y: pushAbove
              ? rect.y - nodeHeight - namespaceGroupGap
              : rect.y + rect.height + namespaceGroupGap,
          }
        } else {
          // Push left or right.
          const pushLeft = node.position.x < rect.x + rect.width / 2
          node.position = {
            x: pushLeft
              ? rect.x - nodeWidth - namespaceGroupGap
              : rect.x + rect.width + namespaceGroupGap,
            y: node.position.y,
          }
        }
        pushed = true
        break // restart rect loop from the beginning
      }

      if (!pushed) break
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
  if (!currentSelectedKey) {
    return ''
  }

  if (snapshot.nodes.some((node) => resourceKey(node.ref) === currentSelectedKey)) {
    return currentSelectedKey
  }

  return ''
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

function serviceReadinessBadge(node: DashboardNode) {
  if (node.ref.kind !== 'Service') {
    return undefined
  }

  const ready = node.attributes?.[serviceReadyEndpointsAttribute]
  const total = node.attributes?.[serviceTotalEndpointsAttribute]
  if (ready === undefined || total === undefined) {
    return undefined
  }

  return `${ready}/${total} ready`
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
  if (apiBaseURL) {
    return `${apiBaseURL}${pathname}`
  }

  return pathname.replace(/^\/+/, '')
}
