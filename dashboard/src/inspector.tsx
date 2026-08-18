import type {ReactNode} from 'react'
import type {DashboardEdge, DashboardNode, DashboardResourceRef} from './types'

interface InspectorItem {
  label: string
  value: string
}

export function hasAttributes(node: DashboardNode) {
  return Object.keys(node.attributes ?? {}).length > 0
}

export function InspectorSection({children, title}: {children: ReactNode; title: string}) {
  return (
    <section className="inspector-section">
      <h3>{title}</h3>
      {children}
    </section>
  )
}

export function InspectorList({items}: {items: InspectorItem[]}) {
  return (
    <div className="inspector-list">
      {items.map((item) => {
        return (
          <article className="inspector-item" key={`${item.label}:${item.value}`}>
            <p>{item.label}</p>
            <strong>{item.value}</strong>
          </article>
        )
      })}
    </div>
  )
}

export function renderAttributes(node: DashboardNode) {
  const entries = Object.entries(node.attributes ?? {})
  if (!entries.length) {
    return <div className="empty-state">No resource attributes were attached.</div>
  }

  return (
    <InspectorList
      items={entries.map(([label, value]) => {
        return {label, value}
      })}
    />
  )
}

function resourceKey(ref: DashboardResourceRef) {
  return [ref.group, ref.kind, ref.namespace, ref.name].join('|')
}

function edgeKey(edge: DashboardEdge) {
  return `${resourceKey(edge.from)}->${resourceKey(edge.to)}:${edge.type}:${edge.detail}`
}

function formatResource(ref: DashboardResourceRef) {
  return ref.namespace ? `${ref.namespace}/${ref.name}` : ref.name
}

function RelationshipGroup({
  direction,
  edges,
  refSide,
}: {
  direction: string
  edges: DashboardEdge[]
  refSide: (edge: DashboardEdge) => DashboardResourceRef
}) {
  if (!edges.length) return null

  return (
    <div className="relationship-group">
      <p className="relationship-direction">{direction}</p>
      <ul className="relationship-list">
        {edges.map((edge) => {
          const ref = refSide(edge)
          return (
            <li className="relationship-row" key={edgeKey(edge)}>
              <span className="relationship-kind">{ref.kind}</span>
              <span className="relationship-label">{edge.detail || edge.type}</span>
              <span className="relationship-target">{formatResource(ref)}</span>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

export function renderRelationships(edges: DashboardEdge[], selectedNode: DashboardNode) {
  if (!edges.length) {
    return <div className="empty-state">No relationships for this resource.</div>
  }

  const selectedNodeKey = resourceKey(selectedNode.ref)
  const incoming: DashboardEdge[] = []
  const outgoing: DashboardEdge[] = []

  for (const edge of edges) {
    if (resourceKey(edge.to) === selectedNodeKey) {
      incoming.push(edge)
    } else {
      outgoing.push(edge)
    }
  }

  return (
    <div className="relationship-groups">
      <RelationshipGroup direction="Outgoing" edges={outgoing} refSide={(e) => e.to} />
      <RelationshipGroup direction="Incoming" edges={incoming} refSide={(e) => e.from} />
    </div>
  )
}

export function renderConditions(node: DashboardNode) {
  const conditions = node.conditions ?? []

  if (!conditions.length) {
    return <div className="empty-state">No Gateway API conditions are present.</div>
  }

  return (
    <div className="condition-list">
      {conditions.map((condition) => (
        <div key={`${condition.type}:${condition.status}:${condition.reason}`} className="condition-card">
          <div className="condition-header">
            <span className="condition-type">{condition.type}</span>
            <span className="condition-status">{condition.status}</span>
          </div>
          {condition.reason && <div className="condition-detail"><span className="condition-label">Reason:</span> {condition.reason}</div>}
          {condition.message && <div className="condition-detail">{condition.message}</div>}
        </div>
      ))}
    </div>
  )
}
