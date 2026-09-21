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

import {
  Background,
  ControlButton,
  Controls,
  Handle,
  ReactFlow,
  Position,
  type Edge,
  type EdgeMouseHandler,
  type Node,
  type NodeMouseHandler,
  type NodeProps,
  type ReactFlowInstance,
} from '@xyflow/react'
import {
  memo,
  startTransition,
  useCallback,
  useDeferredValue,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
} from 'react'
import '@xyflow/react/dist/style.css'
import './App.css'

import {
  applyColorScheme,
  getSystemColorScheme,
  loadStoredColorSchemeMode,
  storeColorSchemeMode,
  watchSystemColorScheme,
  type ColorSchemeMode,
} from './state/colorScheme'
import type {DashboardIssue, DashboardIssuesPayload, DashboardPayload, DashboardResourceRef} from './types'
import {
  apiURL,
  applyCollapsing,
  applyEdgeHover,
  applyFilters,
  buildGraph,
  buildSelectedResourceYAML,
  collapseGroupKey,
  formatResource,
  groupNodeCollapseThreshold,
  isNamespaceGroupNodeId,
  resolveSelectedKey,
  resourceKey,
  visibleGraphSnapshot,
  type GraphOrientation,
  type GroupNodeData,
  type NamespaceGroupNodeData,
} from './graph/graph'
import {loadStoredGraphOrientation, storeGraphOrientation} from './state/layout'
import {buildSummaryCards} from './graph/summary'
import {loadViewStateFromLocation, writeViewStateToURL} from './state/urlState'
import {
  hasAttributes,
  hasDiagnostics,
  InspectorList,
  InspectorSection,
  renderAttributes,
  renderConditions,
  renderDiagnostics,
  renderRelationships,
} from './components/inspector'
import {IssuesPanel} from './components/issues-panel'

type ResourceNodeData = {
  displayName: string
  detailText?: string
  hasNegativeCondition: boolean
  kind: string
  readinessBadge?: string
  sourceBottomHandleCount: number
  sourceTopHandleCount: number
  sourceLeftHandleCount: number
  sourceRightHandleCount: number
  targetBottomHandleCount: number
  targetTopHandleCount: number
  targetLeftHandleCount: number
  targetRightHandleCount: number
}

const nodeTypes = {
  resource: memo(function ResourceNode({data}: NodeProps<Node<ResourceNodeData>>) {
    return (
      <>
        {renderNodeHandles('target', Position.Top, data.targetTopHandleCount)}
        {renderNodeHandles('source', Position.Top, data.sourceTopHandleCount)}
        {renderNodeHandles('target', Position.Left, data.targetLeftHandleCount)}
        {renderNodeHandles('source', Position.Left, data.sourceLeftHandleCount)}
        <article className="flow-node-card">
          {data.hasNegativeCondition ? <span className="flow-node-alert">! Error Status</span> : null}
          <span className="flow-node-kind">{data.kind}</span>
          <strong>{data.displayName}</strong>
          {data.readinessBadge ? (
            <span
              className={
                data.hasNegativeCondition
                  ? 'flow-node-readiness flow-node-readiness-error'
                  : 'flow-node-readiness'
              }
            >
              {data.readinessBadge}
            </span>
          ) : null}
          {data.detailText ? <span className="flow-node-summary">{data.detailText}</span> : null}
        </article>
        {renderNodeHandles('target', Position.Bottom, data.targetBottomHandleCount)}
        {renderNodeHandles('source', Position.Bottom, data.sourceBottomHandleCount)}
        {renderNodeHandles('target', Position.Right, data.targetRightHandleCount)}
        {renderNodeHandles('source', Position.Right, data.sourceRightHandleCount)}
      </>
    )
  }),
  group: memo(function GroupNode({data}: NodeProps<Node<GroupNodeData>>) {
    return (
      <>
        {renderNodeHandles('target', Position.Top, data.targetTopHandleCount)}
        {renderNodeHandles('source', Position.Top, data.sourceTopHandleCount)}
        {renderNodeHandles('target', Position.Left, data.targetLeftHandleCount)}
        {renderNodeHandles('source', Position.Left, data.sourceLeftHandleCount)}
        <article className="flow-node-card flow-node-group">
          <span className="flow-node-kind">{data.kind}</span>
          <strong className="flow-node-group-count">{data.count}</strong>
          {data.namespace ? (
            <span className="flow-node-summary">{data.namespace} · Click to expand</span>
          ) : (
            <span className="flow-node-summary">Click to expand</span>
          )}
        </article>
        {renderNodeHandles('target', Position.Bottom, data.targetBottomHandleCount)}
        {renderNodeHandles('source', Position.Bottom, data.sourceBottomHandleCount)}
        {renderNodeHandles('target', Position.Right, data.targetRightHandleCount)}
        {renderNodeHandles('source', Position.Right, data.sourceRightHandleCount)}
      </>
    )
  }),
  namespaceGroup: memo(function NamespaceGroupNode({data}: NodeProps<Node<NamespaceGroupNodeData>>) {
    return (
      <div className="namespace-group-container">
        <span className="namespace-group-label">{data.namespace}</span>
      </div>
    )
  }),
}


export function App() {
  const [initialViewState] = useState(() => loadViewStateFromLocation())

  const [payload, setPayload] = useState<DashboardPayload | null>(null)
  const [flowNodes, setFlowNodes] = useState<Node[]>([])
  const [flowEdges, setFlowEdges] = useState<Edge[]>([])
  const [selectedKey, setSelectedKey] = useState(initialViewState.selectedKey)
  const [hoveredKey, setHoveredKey] = useState('')
  const [errorMessage, setErrorMessage] = useState('')

  // Issues state.
  const [issues, setIssues] = useState<DashboardIssue[]>([])
  const [issuesOpen, setIssuesOpen] = useState(false)

  // Color scheme: mode is the user's preference ('light' | 'dark' | 'system'),
  // persisted across sessions. systemColorScheme tracks the live OS/browser
  // prefers-color-scheme setting (only consulted when mode is 'system').
  // colorScheme below derives the concrete resolved value from both.
  const [colorSchemeMode, setColorSchemeMode] = useState<ColorSchemeMode>(() => loadStoredColorSchemeMode())
  const [systemColorScheme, setSystemColorScheme] = useState(() => getSystemColorScheme())
  const colorScheme = colorSchemeMode === 'system' ? systemColorScheme : colorSchemeMode

  // Graph layout orientation: 'TB' (top-to-bottom, default) or 'LR'
  // (left-to-right), persisted across sessions.
  const [orientation, setOrientation] = useState<GraphOrientation>(() => loadStoredGraphOrientation())

  const reactFlowInstanceRef = useRef<ReactFlowInstance | null>(null)
  const lastFitOrientationRef = useRef(orientation)
  const pendingOrientationFitRef = useRef(false)
  // Generation counter to discard stale fetch responses. When multiple SSE
  // events fire in quick succession, earlier fetches can resolve after later
  // ones; without this guard the graph would be overwritten with stale data.
  const refreshGenerationRef = useRef(0)

  // Filter state
  const [hiddenKinds, setHiddenKinds] = useState<Set<string>>(initialViewState.hiddenKinds)
  const [namespaceFilter, setNamespaceFilter] = useState(initialViewState.namespaceFilter)
  const [searchFilter, setSearchFilter] = useState(initialViewState.searchFilter)
  const [collapsedKinds, setCollapsedKinds] = useState<Set<string>>(new Set())
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())

  // Derive the set of all kinds and namespaces from the full payload for the filter UI.
  const allKinds = useMemo(() => {
    if (!payload) return []
    const kinds = new Set<string>()
    for (const node of payload.nodes) {
      kinds.add(node.ref.kind)
    }
    return [...kinds].sort()
  }, [payload])

  const allNamespaces = useMemo(() => {
    if (!payload) return []
    const namespaces = new Set<string>()
    for (const node of payload.nodes) {
      if (node.ref.namespace) {
        namespaces.add(node.ref.namespace)
      }
    }
    return [...namespaces].sort()
  }, [payload])

  // Kinds with enough nodes to be collapsible.
  const collapsibleKinds = useMemo(() => {
    if (!payload) return new Set<string>()
    const counts = new Map<string, number>()
    for (const node of payload.nodes) {
      counts.set(node.ref.kind, (counts.get(node.ref.kind) ?? 0) + 1)
    }
    const result = new Set<string>()
    for (const [kind, count] of counts) {
      if (count >= groupNodeCollapseThreshold) result.add(kind)
    }
    return result
  }, [payload])

  // Apply filters and collapsing to produce the graph-ready snapshot.
  const graphPayload = useMemo(() => {
    if (!payload) return null
    const filtered = applyFilters(payload, hiddenKinds, namespaceFilter, searchFilter)
    return applyCollapsing(filtered, collapsedKinds, expandedGroups)
  }, [payload, hiddenKinds, namespaceFilter, searchFilter, collapsedKinds, expandedGroups])

  const deferredSelectedKey = useDeferredValue(selectedKey)
  const selectedNode = deferredSelectedKey
    ? payload?.nodes.find((node) => resourceKey(node.ref) === deferredSelectedKey) ?? null
    : null

  const relatedEdges = payload?.edges.filter((edge) => {
    if (!selectedNode) {
      return false
    }

    const nodeKey = resourceKey(selectedNode.ref)

    return resourceKey(edge.from) === nodeKey || resourceKey(edge.to) === nodeKey
  }) ?? []

  // Resolves the next selected key using the latest selectedKey without needing
  // it as an effect dependency (selectedKey is only read, not reacted to, here).
  const resolveNextSelectedKey = useEffectEvent((currentGraphPayload: DashboardPayload) => {
    return resolveSelectedKey(currentGraphPayload, selectedKey)
  })

  // Rebuild graph whenever graphPayload changes, or the user changes the
  // selection (so the selection glow styling actually gets (re)applied).
  useEffect(() => {
    if (!graphPayload) return

    const nextSelectedKey = resolveNextSelectedKey(graphPayload)
    const graph = buildGraph(graphPayload, nextSelectedKey, colorScheme, orientation)
    const orientationChanged = lastFitOrientationRef.current !== orientation
    lastFitOrientationRef.current = orientation

    if (orientationChanged) {
      pendingOrientationFitRef.current = true
    }

    startTransition(() => {
      setSelectedKey(nextSelectedKey)
      setFlowNodes(graph.nodes)
      setFlowEdges(graph.edges)
    })
  }, [graphPayload, selectedKey, colorScheme, orientation])

  useEffect(() => {
    if (!pendingOrientationFitRef.current) return
    pendingOrientationFitRef.current = false

    void reactFlowInstanceRef.current?.fitView({duration: 300})
  }, [flowNodes])

  const displayedEdges = useMemo(
    () => applyEdgeHover(flowEdges, hoveredKey, colorScheme),
    [flowEdges, hoveredKey, colorScheme],
  )

  useEffect(() => {
    applyColorScheme(colorScheme)
  }, [colorScheme])

  useEffect(() => {
    storeColorSchemeMode(colorSchemeMode)
  }, [colorSchemeMode])

  useEffect(() => {
    return watchSystemColorScheme((scheme) => {
      setSystemColorScheme(scheme)
    })
  }, [])

  useEffect(() => {
    writeViewStateToURL({hiddenKinds, namespaceFilter, searchFilter, selectedKey})
  }, [hiddenKinds, namespaceFilter, searchFilter, selectedKey])

  useEffect(() => {
    storeGraphOrientation(orientation)
  }, [orientation])

  const refreshSnapshot = useEffectEvent(async () => {
    const generation = ++refreshGenerationRef.current

    try {
      const [dataResponse, issuesResponse] = await Promise.all([
        fetch(apiURL('/api/data'), {cache: 'no-store'}),
        fetch(apiURL('/api/issues'), {cache: 'no-store'}),
      ])

      if (generation !== refreshGenerationRef.current) return

      if (!dataResponse.ok) {
        throw new Error(`snapshot request failed with ${dataResponse.status}`)
      }

      const rawPayload = (await dataResponse.json()) as DashboardPayload
      const nextPayload = visibleGraphSnapshot(rawPayload)

      const nextIssues = issuesResponse.ok
        ? ((await issuesResponse.json()) as DashboardIssuesPayload).issues
        : []

      // Auto-collapse kinds that exceed the threshold.
      const kindCounts = new Map<string, number>()
      for (const node of nextPayload.nodes) {
        kindCounts.set(node.ref.kind, (kindCounts.get(node.ref.kind) ?? 0) + 1)
      }
      const autoCollapsed = new Set<string>()
      for (const [kind, count] of kindCounts) {
        if (count >= groupNodeCollapseThreshold) {
          autoCollapsed.add(kind)
        }
      }

      startTransition(() => {
        setPayload(nextPayload)
        setIssues(nextIssues)
        setCollapsedKinds((prev) => {
          // Merge: keep user's existing manual choices, add new auto-collapsed kinds.
          const merged = new Set(prev)
          for (const kind of autoCollapsed) {
            merged.add(kind)
          }
          return merged
        })
        // Reset per-namespace expansions on data refresh so newly auto-collapsed
        // kinds don't have stale expansion overrides.
        setExpandedGroups(new Set())
        setErrorMessage('')
      })
    } catch (error) {
      // Ignore errors from stale requests.
      if (generation !== refreshGenerationRef.current) return

      const message = error instanceof Error ? error.message : String(error)

      startTransition(() => {
        setErrorMessage(message)
      })
    }
  })

  const [, setSSEConnected] = useState(false)

  useEffect(() => {
    void refreshSnapshot()

    const eventSource = new EventSource(apiURL('/events'))

    let debounceTimer: ReturnType<typeof setTimeout> | null = null
    eventSource.addEventListener('changed', () => {
      if (debounceTimer !== null) clearTimeout(debounceTimer)
      debounceTimer = setTimeout(() => {
        debounceTimer = null
        void refreshSnapshot()
      }, 250)
    })

    eventSource.onopen = () => {
      setSSEConnected(true)
    }

    eventSource.onerror = () => {
      setSSEConnected(false)
    }

    return () => {
      if (debounceTimer !== null) clearTimeout(debounceTimer)
      eventSource.close()
    }
  }, [])

  const toggleKind = useCallback((kind: string) => {
    setHiddenKinds((prev) => {
      const next = new Set(prev)
      if (next.has(kind)) {
        next.delete(kind)
      } else {
        next.add(kind)
      }
      return next
    })
  }, [])

  const hideAllKinds = useCallback(() => {
    setHiddenKinds(new Set(allKinds))
  }, [allKinds])

  const showAllKinds = useCallback(() => {
    setHiddenKinds(new Set())
  }, [])

  const collapseAllOfKind = useCallback((kind: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev)
      for (const key of prev) {
        if (key.startsWith(`${kind}|`)) next.delete(key)
      }
      return next.size === prev.size ? prev : next
    })
  }, [])

  const toggleCollapsedKind = useCallback((kind: string) => {
    // If the kind is already globally collapsed but has per-namespace
    // expansions, the first click just re-collapses those expansions.
    // A second click will then toggle the global collapse off.
    const hasExpandedOverrides = [...expandedGroups].some((key) => key.startsWith(`${kind}|`))
    if (collapsedKinds.has(kind) && hasExpandedOverrides) {
      collapseAllOfKind(kind)
      return
    }

    setCollapsedKinds((prev) => {
      const next = new Set(prev)
      if (next.has(kind)) {
        next.delete(kind)
      } else {
        next.add(kind)
      }
      return next
    })
    collapseAllOfKind(kind)
  }, [expandedGroups, collapsedKinds, collapseAllOfKind])


  const handleNodeClick: NodeMouseHandler = (_, node) => {
    if (!graphPayload) {
      return
    }

    // Namespace group containers are not selectable.
    if (isNamespaceGroupNodeId(node.id)) {
      return
    }

    // Clicking a group node expands that specific namespace group.
    if (node.type === 'group') {
      const groupData = node.data as GroupNodeData
      const groupKey = collapseGroupKey(groupData.kind, groupData.namespace ?? '')
      setExpandedGroups((prev) => {
        const next = new Set(prev)
        next.add(groupKey)
        return next
      })
      return
    }

    const nextSelectedKey = node.id === selectedKey ? '' : node.id
    startTransition(() => {
      setSelectedKey(nextSelectedKey)
    })
  }

  const handlePaneClick = useCallback(() => {
    startTransition(() => {
      setSelectedKey('')
    })
  }, [])

  const handleNodeMouseEnter: NodeMouseHandler = (_, node) => {
    if (isNamespaceGroupNodeId(node.id)) return
    setHoveredKey(node.id)
  }

  const handleNodeMouseLeave = useCallback(() => {
    setHoveredKey('')
  }, [])

  const handleEdgeMouseEnter: EdgeMouseHandler = (_, edge) => {
    setHoveredKey(edge.id)
  }

  const handleEdgeMouseLeave = useCallback(() => {
    setHoveredKey('')
  }, [])

  const handleIssueSelect = useCallback((ref: DashboardResourceRef) => {
    startTransition(() => {
      setSelectedKey(resourceKey(ref))
    })
  }, [])

  return (
    <main className="app-shell">
      <section className="hero-panel">
        <div className="hero-copy">
          <p className="eyebrow">Gateway API Dashboard</p>
          <h1>Gateway Lens</h1>
          <p className="lede">
            Visualize your Gateway API topology.
          </p>
        </div>

        <div className="hero-controls">
          <div aria-label="Color theme" className="theme-toggle-group" role="group">
            <button
              aria-pressed={colorScheme === 'light'}
              className="theme-toggle-option"
              onClick={() => setColorSchemeMode('light')}
              title="Light theme"
              type="button"
            >
              ☀️
            </button>
            <button
              aria-pressed={colorScheme === 'dark'}
              className="theme-toggle-option"
              onClick={() => setColorSchemeMode('dark')}
              title="Dark theme"
              type="button"
            >
              🌙
            </button>
          </div>
        </div>
      </section>

      <section className="summary-grid" aria-label="Gateway API resource counts">
        {buildSummaryCards(payload ?? undefined).map((card) => {
          return (
            <article className="summary-card" key={card.label}>
              <p className="summary-label">{card.label}</p>
              <p className="summary-value">{card.value}</p>
            </article>
          )
        })}
      </section>

      {issues.length > 0 ? (
        <section className="issues-bar" aria-label="Detected issues">
          <button
            className="issues-bar-toggle"
            onClick={() => setIssuesOpen((prev) => !prev)}
            type="button"
          >
            <span className="issues-bar-header">
              <span className="issues-bar-title">Issues</span>
              <span className="issues-bar-count">{issues.length}</span>
            </span>
            <span className="issues-bar-chevron">{issuesOpen ? '▾' : '▸'}</span>
          </button>
          {issuesOpen ? (
            <div className="issues-bar-body">
              <IssuesPanel issues={issues} onSelectResource={handleIssueSelect} />
            </div>
          ) : null}
        </section>
      ) : null}

      <section className="filter-bar" aria-label="Graph filters">
        <div className="filter-group">
          <label className="filter-label">Kinds</label>
          <div className="filter-chips">
            <button
              className="filter-chip filter-chip--action"
              disabled={allKinds.length === 0 || hiddenKinds.size === allKinds.length}
              onClick={hideAllKinds}
              type="button"
            >
              Deselect all
            </button>
            <button
              className="filter-chip filter-chip--action"
              disabled={hiddenKinds.size === 0}
              onClick={showAllKinds}
              type="button"
            >
              Select all
            </button>
            {allKinds.map((kind) => {
              const isHidden = hiddenKinds.has(kind)
              const isCollapsible = collapsibleKinds.has(kind)
              const isCollapsed = collapsedKinds.has(kind)
              const kindExpandedNs = isCollapsed
                ? [...expandedGroups]
                    .filter((key) => key.startsWith(`${kind}|`))
                    .map((key) => key.slice(kind.length + 1))
                : []

              return (
                <span className="filter-chip-wrapper" key={kind}>
                  <button
                    className={`filter-chip ${isHidden ? 'filter-chip--hidden' : 'filter-chip--visible'}`}
                    onClick={() => toggleKind(kind)}
                    type="button"
                  >
                    {kind}
                  </button>
                  {isCollapsible && !isHidden ? (
                    <button
                      className={`filter-collapse-toggle ${isCollapsed ? 'filter-collapse-toggle--collapsed' : ''}`}
                      onClick={() => toggleCollapsedKind(kind)}
                      title={isCollapsed ? `Expand ${kind} nodes` : `Collapse ${kind} nodes into a group`}
                      type="button"
                    >
                      {isCollapsed ? '▸' : '▾'}
                    </button>
                  ) : null}
                  {kindExpandedNs.map((ns) => (
                    <button
                      className="filter-expanded-group-chip"
                      key={ns}
                      onClick={() =>
                        setExpandedGroups((prev) => {
                          const next = new Set(prev)
                          next.delete(collapseGroupKey(kind, ns))
                          return next
                        })
                      }
                      title={`Re-collapse ${kind} in ${ns || 'cluster-scoped'}`}
                      type="button"
                    >
                      {ns || '(cluster)'} ✕
                    </button>
                  ))}
                </span>
              )
            })}
          </div>
        </div>
        {allNamespaces.length > 1 ? (
          <div className="filter-group">
            <label className="filter-label" htmlFor="ns-filter">Namespace</label>
            <select
              className="filter-select"
              id="ns-filter"
              onChange={(e) => setNamespaceFilter(e.target.value)}
              value={namespaceFilter}
            >
              <option value="">All namespaces</option>
              {allNamespaces.map((ns) => (
                <option key={ns} value={ns}>{ns}</option>
              ))}
            </select>
          </div>
        ) : null}
        <div className="filter-group">
          <label className="filter-label" htmlFor="search-filter">Search</label>
          <div className="search-control">
            <input
              aria-describedby="search-hint"
              className="filter-input search-input"
              id="search-filter"
              onChange={(e) => setSearchFilter(e.target.value)}
              placeholder="name or namespace/name"
              title="Type any text to match name or namespace. Use namespace/name (e.g. default/my-gateway) to match both."
              type="text"
              value={searchFilter}
            />
          </div>
        </div>

      </section>

      <section className="workspace-grid">
        <section className="canvas-panel">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">Topology</p>
              <h2>Resource Graph</h2>
            </div>
          </div>

          <div className="flow-shell">
            <ReactFlow
              connectOnClick={false}
              edgesReconnectable={false}
              fitView
              edges={displayedEdges}
              nodes={flowNodes}
              nodesConnectable={false}
              nodesDraggable={false}
              nodeTypes={nodeTypes}
              onInit={(instance) => {
                reactFlowInstanceRef.current = instance
              }}
              onEdgeMouseEnter={handleEdgeMouseEnter}
              onEdgeMouseLeave={handleEdgeMouseLeave}
              onNodeClick={handleNodeClick}
              onNodeMouseEnter={handleNodeMouseEnter}
              onNodeMouseLeave={handleNodeMouseLeave}
              onPaneClick={handlePaneClick}
              panOnScroll
              proOptions={{hideAttribution: true}}
            >
              <Background color={colorScheme === 'dark' ? 'rgba(200, 220, 210, 0.1)' : 'rgba(35, 63, 53, 0.12)'} gap={20} />
              <Controls showInteractive={false}>
                <ControlButton
                  aria-label="Left-to-right layout"
                  aria-pressed={orientation === 'LR'}
                  className="orientation-switch-option"
                  onClick={() => setOrientation('LR')}
                  title="Left-to-right layout"
                >
                  <RightArrowIcon />
                </ControlButton>
                <ControlButton
                  aria-label="Top-to-bottom layout"
                  aria-pressed={orientation === 'TB'}
                  className="orientation-switch-option"
                  onClick={() => setOrientation('TB')}
                  title="Top-to-bottom layout"
                >
                  <DownArrowIcon />
                </ControlButton>
              </Controls>
            </ReactFlow>
          </div>

          <section className="resource-yaml-panel" aria-label="Selected resource YAML">
            <div className="panel-header">
              <div>
                <p className="panel-kicker">YAML</p>
                <h3>Selected Resource</h3>
              </div>
            </div>
            <pre className="yaml-preview">
              {selectedNode
                ? buildSelectedResourceYAML(selectedNode)
                : payload
                  ? 'Click a resource in the graph to view its YAML.'
                  : 'Waiting for topology data.'}
            </pre>
          </section>

          {errorMessage ? <p className="banner banner-error">{errorMessage}</p> : null}
        </section>

        <aside className="inspector-panel">
          <div className="panel-header">
            <div>
              <p className="panel-kicker">Inspector</p>
              <h2>{selectedNode ? formatResource(selectedNode.ref) : 'No resource selected'}</h2>
            </div>
          </div>

          {selectedNode ? (
            <>
              <InspectorSection title="Identity">
                <InspectorList
                  items={[
                    {label: 'Kind', value: selectedNode.ref.kind},
                    {label: 'Name', value: selectedNode.ref.name},
                    {label: 'Namespace', value: selectedNode.ref.namespace || '-'},
                    {label: 'API Group', value: selectedNode.ref.group || 'core'},
                  ]}
                />
              </InspectorSection>

              {hasAttributes(selectedNode) ? (
                <InspectorSection title="Attributes">
                  {renderAttributes(selectedNode)}
                </InspectorSection>
              ) : null}

              {selectedNode.ref.kind !== 'Service' ? (
                <InspectorSection title="Conditions">
                  {renderConditions(selectedNode)}
                </InspectorSection>
              ) : null}

              {hasDiagnostics(selectedNode) ? (
                <InspectorSection title="Diagnostics">
                  {renderDiagnostics(selectedNode)}
                </InspectorSection>
              ) : null}

              <InspectorSection title="Relationships">
                {renderRelationships(relatedEdges, selectedNode)}
              </InspectorSection>
            </>
          ) : (
            <div className="empty-state">
              {payload
                ? 'Click a resource in the graph to inspect it.'
                : 'Waiting for topology data.'}
            </div>
          )}
        </aside>
      </section>

    </main>
  )
}

function RightArrowIcon() {
  return (
    <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M2 10v4h12v4l8-6-8-6v4z" />
    </svg>
  )
}

function DownArrowIcon() {
  return (
    <svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
      <path d="M10 2h4v12h4l-6 8-6-8h4z" />
    </svg>
  )
}

function renderNodeHandles(
  type: 'source' | 'target',
  position: Position,
  handleCount: number,
) {
  if (handleCount < 1) {
    return null
  }

  const side = handleSideName(position)
  const offsetProperty = position === Position.Top || position === Position.Bottom ? 'left' : 'top'

  return Array.from({length: handleCount}, (_, index) => {
    return (
      <Handle
        className="resource-handle"
        id={`${type}-${side}-${index}`}
        key={`${type}-${side}-${index}`}
        position={position}
        style={{[offsetProperty]: `${handleOffsetPercent(index, handleCount)}%`}}
        type={type}
      />
    )
  })
}

function handleSideName(position: Position) {
  switch (position) {
    case Position.Top:
      return 'top'
    case Position.Bottom:
      return 'bottom'
    case Position.Left:
      return 'left'
    case Position.Right:
      return 'right'
  }
}

function handleOffsetPercent(handleIndex: number, handleCount: number) {
  const spacing = 100 / (handleCount + 1)

  return Math.round(spacing * (handleIndex + 1) * 100) / 100
}
