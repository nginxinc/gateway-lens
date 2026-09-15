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

// ColorScheme is a fully-resolved theme (no "system" ambiguity).
export type ColorScheme = 'light' | 'dark'

// ColorSchemeMode is the user's preference: an explicit override, or
// "system" to follow the OS/browser prefers-color-scheme setting.
export type ColorSchemeMode = ColorScheme | 'system'

const storageKey = 'gateway-lens:color-scheme-mode'
const darkMediaQuery = '(prefers-color-scheme: dark)'

function isColorSchemeMode(value: unknown): value is ColorSchemeMode {
  return value === 'light' || value === 'dark' || value === 'system'
}

// getSystemColorScheme reads the browser/OS prefers-color-scheme setting.
// Falls back to 'light' in environments without matchMedia support.
export function getSystemColorScheme(): ColorScheme {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return 'light'
  }

  return window.matchMedia(darkMediaQuery).matches ? 'dark' : 'light'
}

// loadStoredColorSchemeMode reads the user's persisted preference, defaulting
// to 'system' when nothing has been stored yet or storage is unavailable.
export function loadStoredColorSchemeMode(): ColorSchemeMode {
  if (typeof window === 'undefined' || !window.localStorage) {
    return 'system'
  }

  try {
    const stored = window.localStorage.getItem(storageKey)
    return isColorSchemeMode(stored) ? stored : 'system'
  } catch {
    return 'system'
  }
}

// storeColorSchemeMode persists the user's preference across sessions.
export function storeColorSchemeMode(mode: ColorSchemeMode): void {
  if (typeof window === 'undefined' || !window.localStorage) {
    return
  }

  try {
    window.localStorage.setItem(storageKey, mode)
  } catch {
    // Ignore storage failures (e.g. private-browsing quota errors).
  }
}

// resolveColorScheme turns a mode ('light' | 'dark' | 'system') into a
// concrete scheme, consulting the OS preference when mode is 'system'.
export function resolveColorScheme(mode: ColorSchemeMode): ColorScheme {
  return mode === 'system' ? getSystemColorScheme() : mode
}

// applyColorScheme reflects the resolved scheme onto the document so CSS can
// react to it via `[data-color-scheme="dark"]` selectors.
export function applyColorScheme(scheme: ColorScheme): void {
  if (typeof document === 'undefined') {
    return
  }

  document.documentElement.dataset.colorScheme = scheme
}

// watchSystemColorScheme invokes onChange whenever the OS/browser
// prefers-color-scheme setting changes, and returns an unsubscribe function.
export function watchSystemColorScheme(onChange: (scheme: ColorScheme) => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => {}
  }

  const mediaQueryList = window.matchMedia(darkMediaQuery)
  const listener = (event: MediaQueryListEvent) => {
    onChange(event.matches ? 'dark' : 'light')
  }

  mediaQueryList.addEventListener('change', listener)
  return () => mediaQueryList.removeEventListener('change', listener)
}
