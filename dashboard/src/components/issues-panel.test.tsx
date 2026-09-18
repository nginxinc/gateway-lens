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

import {fireEvent, render, screen} from '@testing-library/react'
import {describe, expect, it, vi} from 'vitest'

import type {DashboardIssue, DashboardResourceRef} from '../types'
import {
  formatIssueResource,
  IssuesPanel,
  severityClass,
  severityRank,
  sortIssues,
} from './issues-panel'

function makeRef(kind: string, name: string, namespace?: string, group = 'gateway.networking.k8s.io'): DashboardResourceRef {
  return {group, kind, namespace, name}
}

function makeIssue(
  severity: string,
  kind: string,
  name: string,
  reason: string,
  message: string,
  namespace?: string,
): DashboardIssue {
  return {resource: makeRef(kind, name, namespace), severity, reason, message}
}

// ---------------------------------------------------------------------------
// formatIssueResource
// ---------------------------------------------------------------------------

describe('formatIssueResource', () => {
  it('formats namespaced resource', () => {
    expect(formatIssueResource(makeRef('HTTPRoute', 'route-1', 'default'))).toBe('HTTPRoute default/route-1')
  })

  it('formats cluster-scoped resource', () => {
    expect(formatIssueResource(makeRef('GatewayClass', 'my-class'))).toBe('GatewayClass my-class')
  })
})

// ---------------------------------------------------------------------------
// severityRank
// ---------------------------------------------------------------------------

describe('severityRank', () => {
  it('ranks Error highest (0)', () => {
    expect(severityRank('Error')).toBe(0)
  })

  it('ranks Info next (1)', () => {
    expect(severityRank('Info')).toBe(1)
  })

  it('ranks unknown severity lowest (2)', () => {
    expect(severityRank('Other')).toBe(2)
  })
})

// ---------------------------------------------------------------------------
// severityClass
// ---------------------------------------------------------------------------

describe('severityClass', () => {
  it('returns error class for Error', () => {
    expect(severityClass('Error')).toBe('issue-severity--error')
  })

  it('returns info class for Info', () => {
    expect(severityClass('Info')).toBe('issue-severity--info')
  })

  it('returns info class for unknown severity', () => {
    expect(severityClass('Other')).toBe('issue-severity--info')
  })
})

// ---------------------------------------------------------------------------
// sortIssues
// ---------------------------------------------------------------------------

describe('sortIssues', () => {
  it('sorts errors before info', () => {
    const issues: DashboardIssue[] = [
      makeIssue('Info', 'Gateway', 'gw-1', 'SomeInfo', 'info msg', 'default'),
      makeIssue('Error', 'HTTPRoute', 'route-1', 'SomeError', 'err msg', 'default'),
    ]
    const sorted = sortIssues(issues)
    expect(sorted[0].severity).toBe('Error')
    expect(sorted[1].severity).toBe('Info')
  })

  it('sorts same-severity issues alphabetically by resource', () => {
    const issues: DashboardIssue[] = [
      makeIssue('Error', 'HTTPRoute', 'route-b', 'Err', 'msg', 'default'),
      makeIssue('Error', 'Gateway', 'gw-a', 'Err', 'msg', 'default'),
    ]
    const sorted = sortIssues(issues)
    expect(sorted[0].resource.kind).toBe('Gateway')
    expect(sorted[1].resource.kind).toBe('HTTPRoute')
  })

  it('does not mutate the original array', () => {
    const issues: DashboardIssue[] = [
      makeIssue('Info', 'Gateway', 'gw-1', 'I', 'msg', 'default'),
      makeIssue('Error', 'HTTPRoute', 'route-1', 'E', 'msg', 'default'),
    ]
    const original = [...issues]
    sortIssues(issues)
    expect(issues).toEqual(original)
  })
})

// ---------------------------------------------------------------------------
// IssuesPanel
// ---------------------------------------------------------------------------

describe('IssuesPanel', () => {
  it('shows empty message when there are no issues', () => {
    render(<IssuesPanel issues={[]} onSelectResource={() => {}} />)
    expect(screen.getByText('No issues detected.')).toBeTruthy()
  })

  it('renders issues with severity, resource, reason, and message', () => {
    const issues: DashboardIssue[] = [
      makeIssue('Error', 'HTTPRoute', 'route-1', 'TargetNotFound', 'The Gateway does not exist.', 'default'),
    ]
    render(<IssuesPanel issues={issues} onSelectResource={() => {}} />)

    expect(screen.getByText('Error')).toBeTruthy()
    expect(screen.getByText('HTTPRoute default/route-1')).toBeTruthy()
    expect(screen.getByText('TargetNotFound')).toBeTruthy()
    expect(screen.getByText('The Gateway does not exist.')).toBeTruthy()
  })

  it('calls onSelectResource with the issue resource ref when clicked', () => {
    const onSelect = vi.fn()
    const ref = makeRef('Gateway', 'gw-1', 'default')
    const issues: DashboardIssue[] = [
      {resource: ref, severity: 'Error', reason: 'NotAccepted', message: 'Not accepted'},
    ]

    render(<IssuesPanel issues={issues} onSelectResource={onSelect} />)

    fireEvent.click(screen.getByRole('button'))
    expect(onSelect).toHaveBeenCalledOnce()
    expect(onSelect).toHaveBeenCalledWith(ref)
  })

  it('renders multiple issues sorted by severity', () => {
    const issues: DashboardIssue[] = [
      makeIssue('Info', 'Service', 'svc-1', 'I', 'info msg', 'default'),
      makeIssue('Error', 'Gateway', 'gw-1', 'E', 'error msg', 'default'),
    ]
    render(<IssuesPanel issues={issues} onSelectResource={() => {}} />)

    const buttons = screen.getAllByRole('button')
    expect(buttons).toHaveLength(2)
    // First button should be the Error issue.
    expect(buttons[0].textContent).toContain('Error')
    expect(buttons[1].textContent).toContain('Info')
  })
})
