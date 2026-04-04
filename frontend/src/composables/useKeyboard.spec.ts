import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useKeyboard } from './useKeyboard'

describe('useKeyboard', () => {
  const { register, unregister } = useKeyboard()

  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    unregister('g i')
    unregister('j')
    unregister('?')
    vi.useRealTimers()
  })

  it('disables plain shortcuts when input is focused', () => {
    const handler = vi.fn()
    register('j', handler, 'Next', 'inbox')

    const input = document.createElement('input')
    document.body.appendChild(input)
    input.focus()

    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', bubbles: true }))
    expect(handler).not.toHaveBeenCalled()
    input.remove()
  })

  it('resets two-key sequences after timeout', () => {
    const handler = vi.fn()
    register('g i', handler, 'Go inbox', 'global')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
    vi.advanceTimersByTime(2000)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'i' }))

    expect(handler).not.toHaveBeenCalled()
  })

  it('disables sequence shortcuts inside contenteditable regions', () => {
    const handler = vi.fn()
    register('g i', handler, 'Go inbox', 'global')

    const editor = document.createElement('div')
    editor.setAttribute('contenteditable', 'plaintext-only')
    document.body.appendChild(editor)
    editor.focus()

    editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'g', bubbles: true }))
    editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'i', bubbles: true }))

    expect(handler).not.toHaveBeenCalled()
    editor.remove()
  })

  it('disables shift-character shortcuts inside editable targets', () => {
    const handler = vi.fn()
    register('?', handler, 'Help', 'global')

    const textarea = document.createElement('textarea')
    document.body.appendChild(textarea)
    textarea.focus()

    textarea.dispatchEvent(new KeyboardEvent('keydown', { key: '?', shiftKey: true, bubbles: true }))

    expect(handler).not.toHaveBeenCalled()
    textarea.remove()
  })
})
