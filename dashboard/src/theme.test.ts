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

import {nodeColor, relationshipStyle} from './theme'

describe('nodeColor', () => {
  it('returns exact match for GatewayClass', () => {
    const tone = nodeColor('GatewayClass')
    expect(tone.fill).toContain('241, 218, 160')
  })

  it('returns exact match for Gateway', () => {
    const tone = nodeColor('Gateway')
    expect(tone.fill).toContain('183, 218, 208')
  })

  it('returns exact match for ReferenceGrant', () => {
    const tone = nodeColor('ReferenceGrant')
    expect(tone.fill).toContain('240, 209, 188')
  })

  it('returns exact match for Service', () => {
    const tone = nodeColor('Service')
    expect(tone.fill).toContain('230, 222, 210')
  })

  it('returns route color for kinds ending in Route', () => {
    const httpRoute = nodeColor('HTTPRoute')
    const grpcRoute = nodeColor('GRPCRoute')
    const tlsRoute = nodeColor('TLSRoute')
    expect(httpRoute).toEqual(grpcRoute)
    expect(httpRoute).toEqual(tlsRoute)
    expect(httpRoute.fill).toContain('208, 222, 248')
  })

  it('returns policy color for kinds ending in Policy', () => {
    const backendTLS = nodeColor('BackendTLSPolicy')
    const clientTLS = nodeColor('ClientTLSPolicy')
    expect(backendTLS).toEqual(clientTLS)
    expect(backendTLS.fill).toContain('219, 205, 236')
  })

  it('returns default color for unknown kinds', () => {
    const custom = nodeColor('MyCustomCRD')
    expect(custom.fill).toContain('241, 214, 195')
  })

  it('returns all four tone properties', () => {
    const tone = nodeColor('Gateway')
    expect(tone).toHaveProperty('fill')
    expect(tone).toHaveProperty('selectedFill')
    expect(tone).toHaveProperty('selectedStroke')
    expect(tone).toHaveProperty('stroke')
  })

  it('does not match partial kind names', () => {
    // "GatewayClassBinding" is not "GatewayClass" — should fall to default
    const tone = nodeColor('GatewayClassBinding')
    expect(tone.fill).toContain('241, 214, 195')
  })
})

describe('relationshipStyle', () => {
  it('returns consistent style object', () => {
    const style = relationshipStyle()
    expect(style.animated).toBe(false)
    expect(style.lineStyle.strokeWidth).toBe(1.8)
    expect(style.lineStyle.stroke).toBeTruthy()
  })

  it('returns a new object each call', () => {
    const a = relationshipStyle()
    const b = relationshipStyle()
    expect(a).toEqual(b)
    expect(a).not.toBe(b)
  })
})
