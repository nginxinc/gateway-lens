import {
  Background,
  Controls,
  Handle,
  ReactFlow,
  Position,
  type Edge,
  type Node,
  type NodeMouseHandler,
  type NodeProps,
} from '@xyflow/react'
import {
  memo,
  startTransition,
  useCallback,
  useDeferredValue,
  useEffect,
  useEffectEvent,
  useMemo,
  useState,
} from 'react'
import '@xyflow/react/dist/style.css'
import './App.css'

import type {DashboardPayload} from './types'
import {
  apiURL,
  applyCollapsing,
  applyFilters,
  buildGraph,
  buildSelectedResourceYAML,
  formatResource,
  groupNodeCollapseThreshold,
  isNamespaceGroupNodeId,
  resolveSelectedKey,
  resourceKey,
  visibleGraphSnapshot,
  type GroupNodeData,
  type NamespaceGroupNodeData,
} from './graph'
import {buildSummaryCards} from './summary'
import {
  hasAttributes,
  InspectorList,
  InspectorSection,
  renderAttributes,
  renderConditions,
  renderRelationships,
} from './inspector'

type ResourceNodeData = {
  displayName: string
  detailText?: string
  hasNegativeCondition: boolean
  kind: string
  sourceBottomHandleCount: number
  sourceTopHandleCount: number
  targetBottomHandleCount: number
  targetTopHandleCount: number
}

const nodeTypes = {
  resource: memo(function ResourceNode({data}: NodeProps<Node<ResourceNodeData>>) {
    return (
      <>
        {renderNodeHandles('target', Position.Top, data.targetTopHandleCount)}
        {renderNodeHandles('source', Position.Top, data.sourceTopHandleCount)}
        <article className="flow-node-card">
          {data.hasNegativeCondition ? <span className="flow-node-alert">! Error Status</span> : null}
          <span className="flow-node-kind">{data.kind}</span>
          <strong>{data.displayName}</strong>
          {data.detailText ? <span className="flow-node-summary">{data.detailText}</span> : null}
        </article>
        {renderNodeHandles('target', Position.Bottom, data.targetBottomHandleCount)}
        {renderNodeHandles('source', Position.Bottom, data.sourceBottomHandleCount)}
      </>
    )
  }),
  group: memo(function GroupNode({data}: NodeProps<Node<GroupNodeData>>) {
    return (
      <>
        {renderNodeHandles('target', Position.Top, data.targetTopHandleCount)}
        {renderNodeHandles('source', Position.Top, data.sourceTopHandleCount)}
        <article className="flow-node-card flow-node-group">
          <span className="flow-node-kind">{data.kind}</span>
          <strong className="flow-node-group-count">{data.count}</strong>
          <span className="flow-node-summary">Click to expand</span>
        </article>
        {renderNodeHandles('target', Position.Bottom, data.targetBottomHandleCount)}
        {renderNodeHandles('source', Position.Bottom, data.sourceBottomHandleCount)}
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


function App() {
  const [payload, setPayload] = useState<DashboardPayload | null>(null)
  const [flowNodes, setFlowNodes] = useState<Node[]>([])
  const [flowEdges, setFlowEdges] = useState<Edge[]>([])
  const [selectedKey, setSelectedKey] = useState('')
  const [errorMessage, setErrorMessage] = useState('')

  // Filter state
  const [hiddenKinds, setHiddenKinds] = useState<Set<string>>(new Set())
  const [namespaceFilter, setNamespaceFilter] = useState('')
  const [searchFilter, setSearchFilter] = useState('')
  const [collapsedKinds, setCollapsedKinds] = useState<Set<string>>(new Set())

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
    return applyCollapsing(filtered, collapsedKinds)
  }, [payload, hiddenKinds, namespaceFilter, searchFilter, collapsedKinds])

  const deferredSelectedKey = useDeferredValue(selectedKey)
  const selectedNode = payload?.nodes.find((node) => {
    return resourceKey(node.ref) === deferredSelectedKey
  }) ?? payload?.nodes[0] ?? null

  const relatedEdges = payload?.edges.filter((edge) => {
    if (!selectedNode) {
      return false
    }

    const nodeKey = resourceKey(selectedNode.ref)

    return resourceKey(edge.from) === nodeKey || resourceKey(edge.to) === nodeKey
  }) ?? []

  // Rebuild graph whenever graphPayload or selectedKey changes.
  useEffect(() => {
    if (!graphPayload) return

    const nextSelectedKey = resolveSelectedKey(graphPayload, selectedKey)
    const graph = buildGraph(graphPayload, nextSelectedKey)

    startTransition(() => {
      setSelectedKey(nextSelectedKey)
      setFlowNodes(graph.nodes)
      setFlowEdges(graph.edges)
    })
  }, [graphPayload]) // eslint-disable-line react-hooks/exhaustive-deps

  const refreshSnapshot = useEffectEvent(async () => {
    try {
      const response = await fetch(apiURL('/data'), {cache: 'no-store'})
      if (!response.ok) {
        throw new Error(`snapshot request failed with ${response.status}`)
      }

      const rawPayload = (await response.json()) as DashboardPayload
      const nextPayload = visibleGraphSnapshot(rawPayload)

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
        setCollapsedKinds((prev) => {
          // Merge: keep user's existing manual choices, add new auto-collapsed kinds.
          const merged = new Set(prev)
          for (const kind of autoCollapsed) {
            merged.add(kind)
          }
          return merged
        })
        setErrorMessage('')
      })
    } catch (error) {
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

    eventSource.addEventListener('changed', () => {
      void refreshSnapshot()
    })

    eventSource.onopen = () => {
      setSSEConnected(true)
    }

    eventSource.onerror = () => {
      setSSEConnected(false)
    }

    return () => {
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

  const toggleCollapsedKind = useCallback((kind: string) => {
    setCollapsedKinds((prev) => {
      const next = new Set(prev)
      if (next.has(kind)) {
        next.delete(kind)
      } else {
        next.add(kind)
      }
      return next
    })
  }, [])


  const handleNodeClick: NodeMouseHandler = (_, node) => {
    if (!graphPayload) {
      return
    }

    // Namespace group containers are not selectable.
    if (isNamespaceGroupNodeId(node.id)) {
      return
    }

    // Clicking a group node expands that kind.
    if (node.type === 'group') {
      const kind = (node.data as GroupNodeData).kind
      toggleCollapsedKind(kind)
      return
    }

    const nextSelectedKey = node.id
    startTransition(() => {
      setSelectedKey(nextSelectedKey)
    })
  }

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

      <section className="filter-bar" aria-label="Graph filters">
        <div className="filter-group">
          <label className="filter-label">Kinds</label>
          <div className="filter-chips">
            {allKinds.map((kind) => {
              const isHidden = hiddenKinds.has(kind)
              const isCollapsible = collapsibleKinds.has(kind)
              const isCollapsed = collapsedKinds.has(kind)

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
          <input
            className="filter-input"
            id="search-filter"
            onChange={(e) => setSearchFilter(e.target.value)}
            placeholder="Filter by name…"
            type="text"
            value={searchFilter}
          />
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
              edges={flowEdges}
              nodes={flowNodes}
              nodesConnectable={false}
              nodesDraggable={false}
              nodeTypes={nodeTypes}
              onNodeClick={handleNodeClick}
              panOnScroll
              proOptions={{hideAttribution: true}}
            >
              <Background color="rgba(35, 63, 53, 0.12)" gap={20} />
              <Controls showInteractive={false} />
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

              <InspectorSection title="Conditions">
                {renderConditions(selectedNode)}
              </InspectorSection>

              <InspectorSection title="Relationships">
                {renderRelationships(relatedEdges, selectedNode)}
              </InspectorSection>
            </>
          ) : (
            <div className="empty-state">Waiting for topology data.</div>
          )}
        </aside>
      </section>
    </main>
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

  return Array.from({length: handleCount}, (_, index) => {
    const side = position === Position.Top ? 'top' : 'bottom'

    return (
      <Handle
        className="resource-handle"
        id={`${type}-${side}-${index}`}
        key={`${type}-${side}-${index}`}
        position={position}
        style={{left: `${handleOffsetPercent(index, handleCount)}%`}}
        type={type}
      />
    )
  })
}

function handleOffsetPercent(handleIndex: number, handleCount: number) {
  const spacing = 100 / (handleCount + 1)

  return Math.round(spacing * (handleIndex + 1) * 100) / 100
}

export default App
