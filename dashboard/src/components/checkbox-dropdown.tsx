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

import {useEffect, useRef, useState} from 'react'

interface CheckboxDropdownProps {
  id: string
  label: string
  options: string[]
  selected: Set<string>
  onToggle: (option: string) => void
  allLabel?: string
}

// summarizeSelection builds the trigger button's label: a placeholder when
// nothing is selected (meaning "no filter applied"), the single value when
// exactly one option is picked, and a count otherwise.
function summarizeSelection(selected: Set<string>, allLabel: string): string {
  if (selected.size === 0) return allLabel
  if (selected.size === 1) return [...selected][0]
  return `${selected.size} selected`
}

// CheckboxDropdown is a reusable multi-select control: a trigger button
// that opens a popover panel listing one checkbox per option. The panel
// stays open while toggling checkboxes and only closes when the user
// clicks/tabs outside of it, preserving whatever selection was made.
export function CheckboxDropdown({id, label, options, selected, onToggle, allLabel = 'All'}: CheckboxDropdownProps) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return

    function handlePointerDown(event: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setOpen(false)
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') setOpen(false)
    }

    document.addEventListener('mousedown', handlePointerDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handlePointerDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [open])

  return (
    <div className="filter-group" ref={containerRef}>
      <label className="filter-label" htmlFor={id}>{label}</label>
      <div className="checkbox-dropdown">
        <button
          aria-expanded={open}
          aria-haspopup="true"
          className="filter-select checkbox-dropdown-trigger"
          id={id}
          onClick={() => setOpen((prev) => !prev)}
          type="button"
        >
          {summarizeSelection(selected, allLabel)}
        </button>
        {open ? (
          <div className="checkbox-dropdown-panel" role="menu">
            {options.map((option) => (
              <label className="checkbox-dropdown-option" key={option}>
                <input
                  checked={selected.has(option)}
                  onChange={() => onToggle(option)}
                  type="checkbox"
                />
                <span>{option}</span>
              </label>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}
