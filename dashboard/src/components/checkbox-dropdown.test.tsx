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

import {CheckboxDropdown} from './checkbox-dropdown'

describe('CheckboxDropdown', () => {
  it('shows the "all" label when nothing is selected', () => {
    render(
      <CheckboxDropdown
        allLabel="All"
        id="ns-filter"
        label="Namespace"
        onToggle={() => {}}
        options={['default', 'other']}
        selected={new Set()}
      />,
    )
    expect(screen.getByRole('button')).toHaveTextContent('All')
  })

  it('shows the single selected value on the trigger', () => {
    render(
      <CheckboxDropdown
        allLabel="All"
        id="ns-filter"
        label="Namespace"
        onToggle={() => {}}
        options={['default', 'other']}
        selected={new Set(['default'])}
      />,
    )
    expect(screen.getByRole('button')).toHaveTextContent('default')
  })

  it('shows a count when multiple values are selected', () => {
    render(
      <CheckboxDropdown
        allLabel="All"
        id="ns-filter"
        label="Namespace"
        onToggle={() => {}}
        options={['default', 'other']}
        selected={new Set(['default', 'other'])}
      />,
    )
    expect(screen.getByRole('button')).toHaveTextContent('2 selected')
  })

  it('opens the panel with a checkbox per option when the trigger is clicked', () => {
    render(
      <CheckboxDropdown
        allLabel="All"
        id="ns-filter"
        label="Namespace"
        onToggle={() => {}}
        options={['default', 'other']}
        selected={new Set()}
      />,
    )

    expect(screen.queryByRole('menu')).toBeNull()

    fireEvent.click(screen.getByRole('button'))

    expect(screen.getByRole('menu')).toBeTruthy()
    expect(screen.getByLabelText('default')).toBeTruthy()
    expect(screen.getByLabelText('other')).toBeTruthy()
  })

  it('calls onToggle without closing the panel when a checkbox is clicked', () => {
    const onToggle = vi.fn()
    render(
      <CheckboxDropdown
        allLabel="All"
        id="ns-filter"
        label="Namespace"
        onToggle={onToggle}
        options={['default', 'other']}
        selected={new Set()}
      />,
    )

    fireEvent.click(screen.getByRole('button'))
    fireEvent.click(screen.getByLabelText('default'))

    expect(onToggle).toHaveBeenCalledWith('default')
    expect(screen.getByRole('menu')).toBeTruthy()
  })

  it('closes the panel when clicking outside, preserving the selection', () => {
    render(
      <div>
        <div data-testid="outside">outside</div>
        <CheckboxDropdown
          allLabel="All"
          id="ns-filter"
          label="Namespace"
          onToggle={() => {}}
          options={['default', 'other']}
          selected={new Set(['default'])}
        />
      </div>,
    )

    fireEvent.click(screen.getByRole('button'))
    expect(screen.getByRole('menu')).toBeTruthy()

    fireEvent.mouseDown(screen.getByTestId('outside'))

    expect(screen.queryByRole('menu')).toBeNull()
    expect(screen.getByRole('button')).toHaveTextContent('default')
  })

  it('closes the panel when Escape is pressed', () => {
    render(
      <CheckboxDropdown
        allLabel="All"
        id="ns-filter"
        label="Namespace"
        onToggle={() => {}}
        options={['default', 'other']}
        selected={new Set()}
      />,
    )

    fireEvent.click(screen.getByRole('button'))
    expect(screen.getByRole('menu')).toBeTruthy()

    fireEvent.keyDown(document, {key: 'Escape'})

    expect(screen.queryByRole('menu')).toBeNull()
  })
})
