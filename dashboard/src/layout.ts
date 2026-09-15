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

import {defaultGraphOrientation, type GraphOrientation} from './graph'

const storageKey = 'gateway-lens:graph-orientation'

function isGraphOrientation(value: unknown): value is GraphOrientation {
  return value === 'TB' || value === 'LR'
}

// loadStoredGraphOrientation reads the user's persisted layout-orientation
// preference, defaulting to the standard top-to-bottom layout when nothing
// has been stored yet or storage is unavailable.
export function loadStoredGraphOrientation(): GraphOrientation {
  if (typeof window === 'undefined' || !window.localStorage) {
    return defaultGraphOrientation
  }

  try {
    const stored = window.localStorage.getItem(storageKey)
    return isGraphOrientation(stored) ? stored : defaultGraphOrientation
  } catch {
    return defaultGraphOrientation
  }
}

// storeGraphOrientation persists the user's layout-orientation preference
// across sessions.
export function storeGraphOrientation(orientation: GraphOrientation): void {
  if (typeof window === 'undefined' || !window.localStorage) {
    return
  }

  try {
    window.localStorage.setItem(storageKey, orientation)
  } catch {
    // Ignore storage failures (e.g. private-browsing quota errors).
  }
}
