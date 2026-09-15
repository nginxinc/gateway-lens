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
  namespaceFilter: string
  searchFilter: string
  selectedKey: string
}

const hiddenKindsParamName = 'hiddenKinds'
const namespaceParamName = 'ns'
const searchParamName = 'q'
const selectedParamName = 'selected'

// emptyViewState is returned whenever there's no window/URL to read from
// (e.g. during server-side rendering or in a non-DOM test context).
function emptyViewState(): ViewState {
  return {hiddenKinds: new Set(), namespaceFilter: '', searchFilter: '', selectedKey: ''}
}

// encodeViewState turns a ViewState into URLSearchParams, omitting any
// field that's at its default/empty value so unfiltered views produce a
// clean URL with no query string at all.
export function encodeViewState(state: ViewState): URLSearchParams {
  const params = new URLSearchParams()

  if (state.hiddenKinds.size > 0) {
    params.set(hiddenKindsParamName, [...state.hiddenKinds].sort().join(','))
  }
  if (state.namespaceFilter) {
    params.set(namespaceParamName, state.namespaceFilter)
  }
  if (state.searchFilter) {
    params.set(searchParamName, state.searchFilter)
  }
  if (state.selectedKey) {
    params.set(selectedParamName, state.selectedKey)
  }

  return params
}

// decodeViewState parses URLSearchParams back into a ViewState, defaulting
// any missing field to its empty value.
export function decodeViewState(params: URLSearchParams): ViewState {
  const rawHiddenKinds = params.get(hiddenKindsParamName)
  const hiddenKinds = new Set(
    rawHiddenKinds
      ? rawHiddenKinds.split(',').map((kind) => kind.trim()).filter((kind) => kind !== '')
      : [],
  )

  return {
    hiddenKinds,
    namespaceFilter: params.get(namespaceParamName) ?? '',
    searchFilter: params.get(searchParamName) ?? '',
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
