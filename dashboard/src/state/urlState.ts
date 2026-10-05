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

// ViewState is the subset of the dashboard's filter/selection state that
// gets encoded into (and restored from) the URL's query string, so a link
// can be shared that reproduces exactly what the sender was looking at.
export interface ViewState {
  hiddenKinds: Set<string>
  namespaceFilter: Set<string>
  searchFilter: string
  gatewayClassFilter: Set<string>
  selectedKey: string
}

const hiddenKindsParamName = 'hiddenKinds'
const namespaceParamName = 'ns'
const searchParamName = 'q'
const gatewayClassParamName = 'gc'
const selectedParamName = 'selected'

// emptyViewState is returned whenever there's no window/URL to read from
// (e.g. during server-side rendering or in a non-DOM test context).
function emptyViewState(): ViewState {
  return {
    hiddenKinds: new Set(),
    namespaceFilter: new Set(),
    searchFilter: '',
    gatewayClassFilter: new Set(),
    selectedKey: '',
  }
}

// encodeStringSet turns a Set into a sorted, comma-joined string suitable
// for a single URL query parameter.
function encodeStringSet(values: Set<string>): string {
  return [...values].sort().join(',')
}

// decodeStringSet parses a comma-joined query parameter back into a Set,
// ignoring blank entries.
function decodeStringSet(raw: string | null): Set<string> {
  return new Set(raw ? raw.split(',').map((value) => value.trim()).filter((value) => value !== '') : [])
}

// encodeViewState turns a ViewState into URLSearchParams, omitting any
// field that's at its default/empty value so unfiltered views produce a
// clean URL with no query string at all.
export function encodeViewState(state: ViewState): URLSearchParams {
  const params = new URLSearchParams()

  if (state.hiddenKinds.size > 0) {
    params.set(hiddenKindsParamName, encodeStringSet(state.hiddenKinds))
  }
  if (state.namespaceFilter.size > 0) {
    params.set(namespaceParamName, encodeStringSet(state.namespaceFilter))
  }
  if (state.searchFilter) {
    params.set(searchParamName, state.searchFilter)
  }
  if (state.gatewayClassFilter.size > 0) {
    params.set(gatewayClassParamName, encodeStringSet(state.gatewayClassFilter))
  }
  if (state.selectedKey) {
    params.set(selectedParamName, state.selectedKey)
  }

  return params
}

// decodeViewState parses URLSearchParams back into a ViewState, defaulting
// any missing field to its empty value.
export function decodeViewState(params: URLSearchParams): ViewState {
  return {
    hiddenKinds: decodeStringSet(params.get(hiddenKindsParamName)),
    namespaceFilter: decodeStringSet(params.get(namespaceParamName)),
    searchFilter: params.get(searchParamName) ?? '',
    gatewayClassFilter: decodeStringSet(params.get(gatewayClassParamName)),
    selectedKey: params.get(selectedParamName) ?? '',
  }
}

// loadViewStateFromLocation reads the ViewState encoded in the current
// page's URL (if any), used to restore a shared link's view on load.
export function loadViewStateFromLocation(): ViewState {
  if (typeof window === 'undefined' || !window.location) {
    return emptyViewState()
  }

  return decodeViewState(new URLSearchParams(window.location.search))
}

// writeViewStateToURL reflects the given ViewState into the current page's
// URL query string via history.replaceState, keeping the address bar (and
// therefore any copied link) in sync with the live view without adding
// browser-history entries for every filter/selection change.
export function writeViewStateToURL(state: ViewState): void {
  if (typeof window === 'undefined' || typeof window.history?.replaceState !== 'function') {
    return
  }

  const url = new URL(window.location.href)
  url.search = encodeViewState(state).toString()
  window.history.replaceState(window.history.state, '', url)
}
