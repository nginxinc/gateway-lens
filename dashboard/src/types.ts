export interface DashboardPayload {
  generatedAt: string
  nodes: DashboardNode[]
  edges: DashboardEdge[]
  annotations?: DashboardAnnotation[]
}

export interface DashboardNode {
  ref: DashboardResourceRef
  attributes: Record<string, string>
  conditions?: DashboardCondition[]
  manifest?: string
}

export interface DashboardEdge {
  from: DashboardResourceRef
  to: DashboardResourceRef
  type: string
  detail: string
}

export interface DashboardAnnotation {
  ref: DashboardResourceRef
  source: string
  key: string
  value: string
}

export interface DashboardCondition {
  type: string
  status: string
  reason?: string
  message?: string
}

export interface DashboardResourceRef {
  group: string
  kind: string
  namespace?: string
  name: string
}