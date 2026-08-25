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

import {buildSummaryCards} from './summary'
import type {DashboardPayload} from './types'

function makeRef(kind: string, name: string, group = 'gateway.networking.k8s.io') {
  return {group, kind, namespace: 'default', name}
}

function makeNode(kind: string, name: string, group?: string) {
  return {ref: makeRef(kind, name, group), attributes: {}}
}

function makeEdge(
  fromKind: string,
  fromName: string,
  toKind: string,
  toName: string,
  detail: string,
) {
  return {
    from: makeRef(fromKind, fromName),
    to: makeRef(toKind, toName),
    type: 'relationship',
    detail,
  }
}

function makePayload(
  nodes: ReturnType<typeof makeNode>[],
  edges: ReturnType<typeof makeEdge>[] = [],
): DashboardPayload {
  return {generatedAt: new Date().toISOString(), nodes, edges}
}

describe('buildSummaryCards', () => {
  it('returns default cards for undefined snapshot', () => {
    const cards = buildSummaryCards(undefined)
    expect(cards).toEqual([
      {label: 'GatewayClasses', value: 0},
      {label: 'Gateways', value: 0},
      {label: 'Routes', value: 0},
      {label: 'ReferenceGrants', value: 0},
    ])
  })

  it('returns default cards for empty snapshot', () => {
    const cards = buildSummaryCards(makePayload([]))
    expect(cards).toEqual([
      {label: 'GatewayClasses', value: 0},
      {label: 'Gateways', value: 0},
      {label: 'Routes', value: 0},
      {label: 'ReferenceGrants', value: 0},
    ])
  })

  it('counts core resource kinds correctly', () => {
    const cards = buildSummaryCards(
      makePayload([
        makeNode('GatewayClass', 'gc-1'),
        makeNode('GatewayClass', 'gc-2'),
        makeNode('Gateway', 'gw-1'),
        makeNode('HTTPRoute', 'route-1'),
        makeNode('GRPCRoute', 'route-2'),
        makeNode('TLSRoute', 'route-3'),
        makeNode('ReferenceGrant', 'rg-1'),
      ]),
    )

    expect(cards).toEqual([
      {label: 'GatewayClasses', value: 2},
      {label: 'Gateways', value: 1},
      {label: 'Routes', value: 3},
      {label: 'ReferenceGrants', value: 1},
    ])
  })

  it('adds policy cards sorted by kind name', () => {
    const cards = buildSummaryCards(
      makePayload([
        makeNode('Gateway', 'gw-1'),
        makeNode('BackendTLSPolicy', 'btp-1'),
        makeNode('BackendTLSPolicy', 'btp-2'),
        makeNode('ClientTLSPolicy', 'ctp-1'),
      ]),
    )

    // Core cards first, then policies sorted alphabetically
    expect(cards[0]).toEqual({label: 'GatewayClasses', value: 0})
    expect(cards[1]).toEqual({label: 'Gateways', value: 1})
    expect(cards[4]).toEqual({label: 'BackendTLSPolicies', value: 2})
    expect(cards[5]).toEqual({label: 'ClientTLSPolicies', value: 1})
  })

  it('pluralizes correctly', () => {
    const cards = buildSummaryCards(
      makePayload([
        makeNode('GatewayClass', 'gc-1'),
        makeNode('BackendTLSPolicy', 'btp-1'),
      ]),
    )

    const labels = cards.map((c) => c.label)
    // GatewayClass -> GatewayClasses (ends in ss -> +es)
    expect(labels).toContain('GatewayClasses')
    // BackendTLSPolicy -> BackendTLSPolicies (ends in cy -> -y +ies)
    expect(labels).toContain('BackendTLSPolicies')
  })

  it('includes extensionRef CRD kinds', () => {
    const cards = buildSummaryCards(
      makePayload(
        [makeNode('Gateway', 'gw-1'), makeNode('RateLimitFilter', 'rlf-1', 'custom.io')],
        [makeEdge('HTTPRoute', 'route-1', 'RateLimitFilter', 'rlf-1', 'extensionRef')],
      ),
    )

    const labels = cards.map((c) => c.label)
    expect(labels).toContain('RateLimitFilters')

    const extCard = cards.find((c) => c.label === 'RateLimitFilters')
    expect(extCard?.value).toBe(1)
  })

  it('does not add extensionRef card for kinds that are already counted', () => {
    // A Service kind referenced via extensionRef should still only appear under its normal category
    const cards = buildSummaryCards(
      makePayload(
        [makeNode('Gateway', 'gw-1'), makeNode('Service', 'svc-1')],
        [makeEdge('HTTPRoute', 'route-1', 'Service', 'svc-1', 'extensionRef')],
      ),
    )

    // Service is not a core kind that gets its own card (it's not GatewayClass/Gateway/Route/ReferenceGrant/Policy)
    // but it IS counted as an extensionRef
    const serviceCards = cards.filter((c) => c.label === 'Services')
    expect(serviceCards.length).toBe(1)
  })

  it('includes parametersRef CRD kinds', () => {
    const cards = buildSummaryCards(
      makePayload(
        [
          makeNode('GatewayClass', 'gc-1', 'gateway.networking.k8s.io'),
          makeNode('GatewayClassConfig', 'gcc-1', 'config.example.io'),
        ],
        [makeEdge('GatewayClass', 'gc-1', 'GatewayClassConfig', 'gcc-1', 'parametersRef')],
      ),
    )

    const labels = cards.map((c) => c.label)
    expect(labels).toContain('GatewayClassConfigs')

    const paramCard = cards.find((c) => c.label === 'GatewayClassConfigs')
    expect(paramCard?.value).toBe(1)
  })
})
