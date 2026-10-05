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

import {beforeEach, describe, expect, it} from 'vitest'

import {
  decodeViewState,
  encodeViewState,
  loadViewStateFromLocation,
  writeViewStateToURL,
  type ViewState,
} from './urlState'

function emptyState(): ViewState {
  return {
    hiddenKinds: new Set(),
    namespaceFilter: new Set(),
    searchFilter: '',
    gatewayClassFilter: new Set(),
    selectedKey: '',
  }
}

describe('encodeViewState', () => {
  it('produces an empty query string for the default/empty view', () => {
    expect(encodeViewState(emptyState()).toString()).toBe('')
  })

  it('encodes hidden kinds as a sorted comma-joined list', () => {
    const params = encodeViewState({...emptyState(), hiddenKinds: new Set(['Gateway', 'HTTPRoute'])})
    expect(params.get('hiddenKinds')).toBe('Gateway,HTTPRoute')
  })

  it('sorts hidden kinds regardless of insertion order', () => {
    const params = encodeViewState({...emptyState(), hiddenKinds: new Set(['HTTPRoute', 'Gateway'])})
    expect(params.get('hiddenKinds')).toBe('Gateway,HTTPRoute')
  })

  it('encodes namespace, search, and selection filters', () => {
    const params = encodeViewState({
      hiddenKinds: new Set(),
      namespaceFilter: new Set(['default']),
      searchFilter: 'my-gateway',
      gatewayClassFilter: new Set(),
      selectedKey: 'gateway.networking.k8s.io|Gateway|default|my-gateway',
    })
    expect(params.get('ns')).toBe('default')
    expect(params.get('q')).toBe('my-gateway')
    expect(params.get('selected')).toBe('gateway.networking.k8s.io|Gateway|default|my-gateway')
  })

  it('encodes multiple namespaces as a sorted comma-joined list', () => {
    const params = encodeViewState({...emptyState(), namespaceFilter: new Set(['prod', 'default'])})
    expect(params.get('ns')).toBe('default,prod')
  })

  it('encodes the GatewayClass filter', () => {
    const params = encodeViewState({...emptyState(), gatewayClassFilter: new Set(['nginx'])})
    expect(params.get('gc')).toBe('nginx')
  })

  it('encodes multiple GatewayClasses as a sorted comma-joined list', () => {
    const params = encodeViewState({...emptyState(), gatewayClassFilter: new Set(['nginx', 'envoy'])})
    expect(params.get('gc')).toBe('envoy,nginx')
  })

  it('omits empty fields entirely rather than encoding them as empty strings', () => {
    const params = encodeViewState({...emptyState(), namespaceFilter: new Set(['default'])})
    expect(params.has('q')).toBe(false)
    expect(params.has('gc')).toBe(false)
    expect(params.has('selected')).toBe(false)
    expect(params.has('hiddenKinds')).toBe(false)
  })
})

describe('decodeViewState', () => {
  it('returns the empty state for an empty query string', () => {
    expect(decodeViewState(new URLSearchParams(''))).toEqual(emptyState())
  })

  it('parses hidden kinds back into a Set', () => {
    const state = decodeViewState(new URLSearchParams('hiddenKinds=Gateway,HTTPRoute'))
    expect(state.hiddenKinds).toEqual(new Set(['Gateway', 'HTTPRoute']))
  })

  it('ignores blank entries when parsing hidden kinds', () => {
    const state = decodeViewState(new URLSearchParams('hiddenKinds=Gateway,,HTTPRoute,'))
    expect(state.hiddenKinds).toEqual(new Set(['Gateway', 'HTTPRoute']))
  })

  it('parses namespace, search, and selection filters', () => {
    const state = decodeViewState(new URLSearchParams('ns=default&q=my-gateway&selected=foo'))
    expect(state.namespaceFilter).toEqual(new Set(['default']))
    expect(state.searchFilter).toBe('my-gateway')
    expect(state.selectedKey).toBe('foo')
  })

  it('parses multiple namespaces into a Set', () => {
    const state = decodeViewState(new URLSearchParams('ns=default,prod'))
    expect(state.namespaceFilter).toEqual(new Set(['default', 'prod']))
  })

  it('parses the GatewayClass filter', () => {
    const state = decodeViewState(new URLSearchParams('gc=nginx'))
    expect(state.gatewayClassFilter).toEqual(new Set(['nginx']))
  })

  it('parses multiple GatewayClasses into a Set', () => {
    const state = decodeViewState(new URLSearchParams('gc=nginx,envoy'))
    expect(state.gatewayClassFilter).toEqual(new Set(['nginx', 'envoy']))
  })

  it('round-trips through encodeViewState', () => {
    const original: ViewState = {
      hiddenKinds: new Set(['Gateway']),
      namespaceFilter: new Set(['default']),
      searchFilter: 'demo',
      gatewayClassFilter: new Set(['nginx']),
      selectedKey: 'group|Gateway|default|demo',
    }
    expect(decodeViewState(encodeViewState(original))).toEqual(original)
  })

  it('round-trips multiple namespaces and GatewayClasses', () => {
    const original: ViewState = {
      hiddenKinds: new Set(),
      namespaceFilter: new Set(['default', 'prod']),
      searchFilter: '',
      gatewayClassFilter: new Set(['nginx', 'envoy']),
      selectedKey: '',
    }
    expect(decodeViewState(encodeViewState(original))).toEqual(original)
  })
})

describe('loadViewStateFromLocation / writeViewStateToURL', () => {
  beforeEach(() => {
    window.history.replaceState(null, '', '/')
  })

  it('loads the empty state when the URL has no query string', () => {
    expect(loadViewStateFromLocation()).toEqual(emptyState())
  })

  it('writes the view state into the URL and reads it back', () => {
    const state: ViewState = {
      hiddenKinds: new Set(['Gateway']),
      namespaceFilter: new Set(['default']),
      searchFilter: 'demo',
      gatewayClassFilter: new Set(['nginx']),
      selectedKey: 'group|Gateway|default|demo',
    }

    writeViewStateToURL(state)

    expect(window.location.search).toBe(
      '?hiddenKinds=Gateway&ns=default&q=demo&gc=nginx&selected=group%7CGateway%7Cdefault%7Cdemo',
    )
    expect(loadViewStateFromLocation()).toEqual(state)
  })

  it('clears the query string when writing the empty state', () => {
    writeViewStateToURL({
      hiddenKinds: new Set(['Gateway']),
      namespaceFilter: new Set(['default']),
      searchFilter: '',
      gatewayClassFilter: new Set(),
      selectedKey: '',
    })
    writeViewStateToURL(emptyState())

    expect(window.location.search).toBe('')
  })

  it('uses replaceState rather than pushState (no new history entry)', () => {
    const initialLength = window.history.length
    writeViewStateToURL({...emptyState(), namespaceFilter: new Set(['default'])})
    writeViewStateToURL({...emptyState(), namespaceFilter: new Set(['other'])})
    expect(window.history.length).toBe(initialLength)
  })
})
