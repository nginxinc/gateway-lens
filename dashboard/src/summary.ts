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

import type {DashboardPayload} from './types'

export interface SummaryCard {
  label: string
  value: number
}

function pluralKind(kind: string): string {
  if (kind.endsWith('ss')) return kind + 'es' // GatewayClass -> GatewayClasses
  if (kind.endsWith('cy')) return kind.slice(0, -1) + 'ies' // BackendTLSPolicy -> BackendTLSPolicies
  return kind + 's'
}

function sortedEntries(map: Map<string, number>): [string, number][] {
  return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]))
}

const defaultCards: SummaryCard[] = [
  {label: 'GatewayClasses', value: 0},
  {label: 'Gateways', value: 0},
  {label: 'Routes', value: 0},
  {label: 'ReferenceGrants', value: 0},
]

export function buildSummaryCards(snapshot: DashboardPayload | undefined): SummaryCard[] {
  if (!snapshot) {
    return defaultCards
  }

  // Single pass over nodes to count all kinds at once.
  let gatewayClassCount = 0
  let gatewayCount = 0
  let routeCount = 0
  let referenceGrantCount = 0
  const policyCounts = new Map<string, number>()
  const allKindCounts = new Map<string, number>()

  for (const node of snapshot.nodes) {
    const kind = node.ref.kind
    allKindCounts.set(kind, (allKindCounts.get(kind) ?? 0) + 1)

    if (kind === 'GatewayClass') gatewayClassCount++
    else if (kind === 'Gateway') gatewayCount++
    else if (kind.endsWith('Route')) routeCount++
    else if (kind === 'ReferenceGrant') referenceGrantCount++
    else if (kind.endsWith('Policy')) {
      policyCounts.set(kind, (policyCounts.get(kind) ?? 0) + 1)
    }
  }

  const cards: SummaryCard[] = [
    {label: 'GatewayClasses', value: gatewayClassCount},
    {label: 'Gateways', value: gatewayCount},
    {label: 'Routes', value: routeCount},
    {label: 'ReferenceGrants', value: referenceGrantCount},
  ]

  // Add a card for each distinct policy kind.
  for (const [kind, count] of sortedEntries(policyCounts)) {
    cards.push({label: pluralKind(kind), value: count})
  }

  // Collect each distinct ExtensionRef and ParametersRef CRD kind into its own card.
  const dynamicRefKinds = new Set<string>()
  for (const edge of snapshot.edges) {
    if (edge.detail === 'extensionRef' || edge.detail === 'parametersRef') {
      dynamicRefKinds.add(edge.to.kind)
    }
  }
  if (dynamicRefKinds.size > 0) {
    for (const [kind, count] of sortedEntries(allKindCounts)) {
      if (dynamicRefKinds.has(kind)) {
        cards.push({label: pluralKind(kind), value: count})
      }
    }
  }

  return cards
}
