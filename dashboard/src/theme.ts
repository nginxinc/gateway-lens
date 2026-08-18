interface ColorTone {
  fill: string
  selectedFill: string
  selectedStroke: string
  stroke: string
}

const kindColors: Record<string, ColorTone> = {
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

const routeColor: ColorTone = {
  fill: 'rgba(208, 222, 248, 0.9)',
  selectedFill: 'rgba(208, 222, 248, 1)',
  selectedStroke: 'rgba(33, 81, 149, 1)',
  stroke: 'rgba(33, 81, 149, 0.45)',
}

const policyColor: ColorTone = {
  fill: 'rgba(219, 205, 236, 0.9)',
  selectedFill: 'rgba(219, 205, 236, 1)',
  selectedStroke: 'rgba(97, 58, 145, 1)',
  stroke: 'rgba(97, 58, 145, 0.4)',
}

const defaultColor: ColorTone = {
  fill: 'rgba(241, 214, 195, 0.9)',
  selectedFill: 'rgba(241, 214, 195, 1)',
  selectedStroke: 'rgba(156, 88, 46, 1)',
  stroke: 'rgba(156, 88, 46, 0.4)',
}

export function nodeColor(kind: string): ColorTone {
  const exact = kindColors[kind]
  if (exact) return exact

  if (kind.endsWith('Route')) return routeColor
  if (kind.endsWith('Policy')) return policyColor

  return defaultColor
}

export function relationshipStyle() {
  return {
    animated: false,
    lineStyle: {
      stroke: 'rgba(81, 104, 93, 0.62)',
      strokeWidth: 1.8,
    },
  }
}
