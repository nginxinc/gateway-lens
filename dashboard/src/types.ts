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