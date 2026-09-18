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

import type {DashboardIssue, DashboardResourceRef} from '../types'

export function formatIssueResource(ref: DashboardResourceRef): string {
  const name = ref.namespace ? `${ref.namespace}/${ref.name}` : ref.name
  return `${ref.kind} ${name}`
}

export function severityRank(severity: string): number {
  switch (severity) {
    case 'Error':
      return 0
    case 'Info':
      return 1
    default:
      return 2
  }
}

export function sortIssues(issues: DashboardIssue[]): DashboardIssue[] {
  return [...issues].sort((a, b) => {
    const sev = severityRank(a.severity) - severityRank(b.severity)
    if (sev !== 0) return sev
    return formatIssueResource(a.resource).localeCompare(formatIssueResource(b.resource))
  })
}

export function severityClass(severity: string): string {
  switch (severity) {
    case 'Error':
      return 'issue-severity--error'
    case 'Info':
      return 'issue-severity--info'
    default:
      return 'issue-severity--info'
  }
}

export function IssuesPanel({
  issues,
  onSelectResource,
}: {
  issues: DashboardIssue[]
  onSelectResource: (ref: DashboardResourceRef) => void
}) {
  const sorted = sortIssues(issues)

  if (sorted.length === 0) {
    return (
      <div className="issues-panel-empty">
        No issues detected.
      </div>
    )
  }

  return (
    <ul className="issues-list">
      {sorted.map((issue, idx) => (
        <li key={`${formatIssueResource(issue.resource)}:${issue.reason}:${idx}`} className="issue-row">
          <button
            className="issue-row-button"
            onClick={() => onSelectResource(issue.resource)}
            title={`Select ${formatIssueResource(issue.resource)}`}
            type="button"
          >
            <span className={`issue-severity ${severityClass(issue.severity)}`}>
              {issue.severity}
            </span>
            <span className="issue-resource">{formatIssueResource(issue.resource)}</span>
            <span className="issue-reason">{issue.reason}</span>
            <span className="issue-message">{issue.message}</span>
          </button>
        </li>
      ))}
    </ul>
  )
}
