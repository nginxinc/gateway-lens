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

import type {ColorScheme} from './colorScheme'

export interface ColorTone {
  fill: string
  selectedFill: string
  selectedStroke: string
  stroke: string
}

const lightKindColors: Record<string, ColorTone> = {
  GatewayClass: {
    fill: 'rgba(241, 218, 160, 0.86)',
    selectedFill: 'rgba(241, 218, 160, 1)',
    selectedStroke: 'rgba(142, 101, 28, 1)',
    stroke: 'rgba(142, 101, 28, 0.5)',
  },
  Gateway: {
    fill: 'rgba(183, 218, 208, 0.88)',
    selectedFill: 'rgba(183, 218, 208, 1)',
    selectedStroke: 'rgba(27, 104, 87, 1)',
    stroke: 'rgba(27, 104, 87, 0.45)',
  },
  ReferenceGrant: {
    fill: 'rgba(240, 209, 188, 0.9)',
    selectedFill: 'rgba(240, 209, 188, 1)',
    selectedStroke: 'rgba(145, 77, 35, 1)',
    stroke: 'rgba(145, 77, 35, 0.45)',
  },
  Service: {
    fill: 'rgba(230, 222, 210, 0.9)',
    selectedFill: 'rgba(230, 222, 210, 1)',
    selectedStroke: 'rgba(120, 100, 72, 1)',
    stroke: 'rgba(120, 100, 72, 0.4)',
  },
}

const lightRouteColor: ColorTone = {
  fill: 'rgba(208, 222, 248, 0.9)',
  selectedFill: 'rgba(208, 222, 248, 1)',
  selectedStroke: 'rgba(33, 81, 149, 1)',
  stroke: 'rgba(33, 81, 149, 0.45)',
}

const lightPolicyColor: ColorTone = {
  fill: 'rgba(219, 205, 236, 0.9)',
  selectedFill: 'rgba(219, 205, 236, 1)',
  selectedStroke: 'rgba(97, 58, 145, 1)',
  stroke: 'rgba(97, 58, 145, 0.4)',
}

const lightDefaultColor: ColorTone = {
  fill: 'rgba(241, 214, 195, 0.9)',
  selectedFill: 'rgba(241, 214, 195, 1)',
  selectedStroke: 'rgba(156, 88, 46, 1)',
  stroke: 'rgba(156, 88, 46, 0.4)',
}

const darkKindColors: Record<string, ColorTone> = {
  GatewayClass: {
    fill: 'rgba(120, 98, 42, 0.55)',
    selectedFill: 'rgba(150, 122, 52, 0.9)',
    selectedStroke: 'rgba(245, 200, 100, 1)',
    stroke: 'rgba(245, 200, 100, 0.5)',
  },
  Gateway: {
    fill: 'rgba(38, 92, 76, 0.55)',
    selectedFill: 'rgba(45, 112, 92, 0.9)',
    selectedStroke: 'rgba(110, 224, 190, 1)',
    stroke: 'rgba(110, 224, 190, 0.45)',
  },
  ReferenceGrant: {
    fill: 'rgba(110, 68, 34, 0.55)',
    selectedFill: 'rgba(135, 84, 40, 0.9)',
    selectedStroke: 'rgba(240, 175, 120, 1)',
    stroke: 'rgba(240, 175, 120, 0.45)',
  },
  Service: {
    fill: 'rgba(80, 76, 64, 0.55)',
    selectedFill: 'rgba(98, 93, 78, 0.9)',
    selectedStroke: 'rgba(220, 208, 180, 1)',
    stroke: 'rgba(220, 208, 180, 0.4)',
  },
}

const darkRouteColor: ColorTone = {
  fill: 'rgba(40, 66, 108, 0.55)',
  selectedFill: 'rgba(48, 80, 130, 0.9)',
  selectedStroke: 'rgba(140, 185, 245, 1)',
  stroke: 'rgba(140, 185, 245, 0.45)',
}

const darkPolicyColor: ColorTone = {
  fill: 'rgba(76, 54, 100, 0.55)',
  selectedFill: 'rgba(92, 66, 122, 0.9)',
  selectedStroke: 'rgba(200, 170, 235, 1)',
  stroke: 'rgba(200, 170, 235, 0.4)',
}

const darkDefaultColor: ColorTone = {
  fill: 'rgba(108, 76, 50, 0.55)',
  selectedFill: 'rgba(132, 92, 60, 0.9)',
  selectedStroke: 'rgba(235, 185, 140, 1)',
  stroke: 'rgba(235, 185, 140, 0.4)',
}

export function nodeColor(kind: string, scheme: ColorScheme = 'light'): ColorTone {
  const kindColors = scheme === 'dark' ? darkKindColors : lightKindColors
  const routeColor = scheme === 'dark' ? darkRouteColor : lightRouteColor
  const policyColor = scheme === 'dark' ? darkPolicyColor : lightPolicyColor
  const defaultColor = scheme === 'dark' ? darkDefaultColor : lightDefaultColor

  const exact = kindColors[kind]
  if (exact) return exact

  if (kind.endsWith('Route')) return routeColor
  if (kind.endsWith('Policy')) return policyColor

  return defaultColor
}

export function relationshipStyle(scheme: ColorScheme = 'light') {
  return {
    animated: false,
    lineStyle: {
      stroke: scheme === 'dark' ? 'rgba(150, 168, 160, 0.6)' : 'rgba(81, 104, 93, 0.62)',
      strokeWidth: 1.8,
    },
  }
}
