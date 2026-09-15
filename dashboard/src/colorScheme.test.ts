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

import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'

import {
  applyColorScheme,
  getSystemColorScheme,
  loadStoredColorSchemeMode,
  resolveColorScheme,
  storeColorSchemeMode,
  watchSystemColorScheme,
} from './colorScheme'

type ChangeListener = (event: MediaQueryListEvent) => void

// Minimal mock of the MediaQueryList interface returned by window.matchMedia,
// with the ability to simulate a prefers-color-scheme change from tests.
function installMatchMediaMock(initialMatches: boolean) {
  const listeners = new Set<ChangeListener>()
  let matches = initialMatches

  const mediaQueryList: Pick<MediaQueryList, 'matches' | 'media' | 'addEventListener' | 'removeEventListener'> = {
    get matches() {
      return matches
    },
    media: '(prefers-color-scheme: dark)',
    addEventListener: ((_type: string, listener: ChangeListener) => {
      listeners.add(listener)
    }) as MediaQueryList['addEventListener'],
    removeEventListener: ((_type: string, listener: ChangeListener) => {
      listeners.delete(listener)
    }) as MediaQueryList['removeEventListener'],
  }

  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue(mediaQueryList))

  return {
    listenerCount: () => listeners.size,
    simulateChange: (nextMatches: boolean) => {
      matches = nextMatches
      const event = {matches: nextMatches} as MediaQueryListEvent
      for (const listener of listeners) {
        listener(event)
      }
    },
  }
}

// Simulates an environment without matchMedia support (e.g. older browsers).
function removeMatchMedia() {
  vi.stubGlobal('matchMedia', undefined)
}

describe('getSystemColorScheme', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns dark when the OS prefers dark', () => {
    installMatchMediaMock(true)
    expect(getSystemColorScheme()).toBe('dark')
  })

  it('returns light when the OS prefers light', () => {
    installMatchMediaMock(false)
    expect(getSystemColorScheme()).toBe('light')
  })

  it('defaults to light when matchMedia is unavailable', () => {
    removeMatchMedia()
    expect(getSystemColorScheme()).toBe('light')
  })
})

describe('loadStoredColorSchemeMode / storeColorSchemeMode', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('defaults to system when nothing has been stored', () => {
    expect(loadStoredColorSchemeMode()).toBe('system')
  })

  it('round-trips a stored light mode', () => {
    storeColorSchemeMode('light')
    expect(loadStoredColorSchemeMode()).toBe('light')
  })

  it('round-trips a stored dark mode', () => {
    storeColorSchemeMode('dark')
    expect(loadStoredColorSchemeMode()).toBe('dark')
  })

  it('round-trips a stored system mode', () => {
    storeColorSchemeMode('system')
    expect(loadStoredColorSchemeMode()).toBe('system')
  })

  it('falls back to system for garbage stored values', () => {
    window.localStorage.setItem('gateway-lens:color-scheme-mode', 'not-a-real-mode')
    expect(loadStoredColorSchemeMode()).toBe('system')
  })
})

describe('resolveColorScheme', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('passes explicit light/dark modes through unchanged', () => {
    expect(resolveColorScheme('light')).toBe('light')
    expect(resolveColorScheme('dark')).toBe('dark')
  })

  it('resolves system mode using the OS preference', () => {
    installMatchMediaMock(true)
    expect(resolveColorScheme('system')).toBe('dark')

    installMatchMediaMock(false)
    expect(resolveColorScheme('system')).toBe('light')
  })
})

describe('applyColorScheme', () => {
  afterEach(() => {
    delete document.documentElement.dataset.colorScheme
  })

  it('sets the data-color-scheme attribute on the document element', () => {
    applyColorScheme('dark')
    expect(document.documentElement.dataset.colorScheme).toBe('dark')

    applyColorScheme('light')
    expect(document.documentElement.dataset.colorScheme).toBe('light')
  })
})

describe('watchSystemColorScheme', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('invokes the callback when the OS preference changes', () => {
    const mock = installMatchMediaMock(false)
    const onChange = vi.fn()

    const unsubscribe = watchSystemColorScheme(onChange)
    expect(mock.listenerCount()).toBe(1)

    mock.simulateChange(true)
    expect(onChange).toHaveBeenCalledWith('dark')

    mock.simulateChange(false)
    expect(onChange).toHaveBeenCalledWith('light')

    unsubscribe()
    expect(mock.listenerCount()).toBe(0)
  })

  it('returns a no-op unsubscribe when matchMedia is unavailable', () => {
    removeMatchMedia()

    const unsubscribe = watchSystemColorScheme(vi.fn())
    expect(() => unsubscribe()).not.toThrow()
  })
})
