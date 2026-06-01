export type DashboardPayload = {
  generatedAt: string
  nodes: DashboardNode[]
  edges: DashboardEdge[]
  annotations?: DashboardAnnotation[]
}

export type DashboardNode = {
  ref: DashboardResourceRef
  attributes: Record<string, string>
  conditions?: DashboardCondition[]
  manifest?: string
}

export type DashboardEdge = {
  from: DashboardResourceRef
  to: DashboardResourceRef
  type: string
  detail: string
}

export type DashboardAnnotation = {
  ref: DashboardResourceRef
  source: string
  key: string
  value: string
}

export type DashboardCondition = {
  type: string
  status: string
  reason?: string
  message?: string
}

export type DashboardResourceRef = {
  group: string
  kind: string
  namespace?: string
  name: string
}