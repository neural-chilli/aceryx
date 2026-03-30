import { computed, shallowRef } from 'vue'

export type ShortcutHandler = (event: KeyboardEvent) => void

export type ShortcutDef = {
  keys: string
  handler: ShortcutHandler
  description: string
  scope: string
}

const shortcuts = shallowRef<Map<string, ShortcutDef>>(new Map())
const isMac = typeof navigator !== 'undefined' ? /mac/i.test(navigator.platform) : false

let listenerInstalled = false
let sequenceFirst: string | null = null
let sequenceTimer: number | null = null

function normalizeKeyName(raw: string): string {
  const key = raw.trim().toLowerCase()
  switch (key) {
    case 'cmd':
    case 'meta':
    case 'command':
    case 'ctrl':
    case 'control':
      return 'mod'
    case 'esc':
      return 'escape'
    case 'return':
      return 'enter'
    default:
      return key
  }
}

function normalizeChord(raw: string): string {
  const pieces = raw
    .split('+')
    .map((part) => normalizeKeyName(part))
    .filter((part) => part.length > 0)

  const mods = ['mod', 'alt', 'shift']
  const ordered: string[] = []
  for (const mod of mods) {
    if (pieces.includes(mod)) {
      ordered.push(mod)
    }
  }
  const key = pieces.find((part) => !mods.includes(part))
  if (key) {
    ordered.push(key)
  }
  return ordered.join('+')
}

function normalizeShortcut(raw: string): string {
  return raw
    .trim()
    .toLowerCase()
    .split(' ')
    .map((chunk) => normalizeChord(chunk))
    .filter((chunk) => chunk.length > 0)
    .join(' ')
}

function eventToChord(event: KeyboardEvent): string {
  const key = normalizeKeyName(event.key === ' ' ? 'space' : event.key)
  const parts: string[] = []
  if (event.metaKey || event.ctrlKey) {
    parts.push('mod')
  }
  if (event.altKey) {
    parts.push('alt')
  }
  if (event.shiftKey && key !== '?') {
    parts.push('shift')
  }
  parts.push(key)
  return normalizeChord(parts.join('+'))
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!target || !(target instanceof HTMLElement)) {
    return false
  }
  const el = target as HTMLElement
  if (el.isContentEditable) {
    return true
  }
  const tag = String(el.tagName || '').toLowerCase()
  if (tag === 'input' || tag === 'textarea' || tag === 'select') {
    return true
  }
  return Boolean(el.closest('[contenteditable="true"]'))
}

function isCharacterKey(chord: string): boolean {
  const key = chord.split('+').at(-1) ?? ''
  return key.length === 1
}

function hasModifier(chord: string): boolean {
  return chord.includes('mod+') || chord.includes('alt+') || chord.includes('shift+')
}

function clearSequence() {
  sequenceFirst = null
  if (sequenceTimer !== null) {
    window.clearTimeout(sequenceTimer)
    sequenceTimer = null
  }
}

function firstSequenceParts(): Set<string> {
  const set = new Set<string>()
  for (const key of shortcuts.value.keys()) {
    const parts = key.split(' ')
    if (parts.length === 2) {
      set.add(parts[0])
    }
  }
  return set
}

function installListener() {
  if (listenerInstalled || typeof window === 'undefined') {
    return
  }
  listenerInstalled = true
  window.addEventListener('keydown', (event: KeyboardEvent) => {
    const chord = eventToChord(event)
    const editable = isEditableTarget(event.target)
    if (editable && !hasModifier(chord) && isCharacterKey(chord)) {
      return
    }

    if (sequenceFirst) {
      const combo = `${sequenceFirst} ${chord}`
      const seq = shortcuts.value.get(combo)
      if (seq) {
        event.preventDefault()
        clearSequence()
        seq.handler(event)
        return
      }
      clearSequence()
    }

    const direct = shortcuts.value.get(chord)
    if (direct) {
      event.preventDefault()
      direct.handler(event)
      return
    }

    if (firstSequenceParts().has(chord)) {
      sequenceFirst = chord
      if (sequenceTimer !== null) {
        window.clearTimeout(sequenceTimer)
      }
      sequenceTimer = window.setTimeout(() => {
        clearSequence()
      }, 1000)
    }
  })
}

function prettyShortcut(keys: string): string {
  const normalized = normalizeShortcut(keys)
  const chunks = normalized.split(' ')
  return chunks
    .map((chunk) => {
      const parts = chunk.split('+')
      const output = parts.map((part) => {
        switch (part) {
          case 'mod':
            return isMac ? '⌘' : 'Ctrl'
          case 'enter':
            return isMac ? '↵' : 'Enter'
          case 'shift':
            return isMac ? '⇧' : 'Shift'
          case 'escape':
            return isMac ? 'Esc' : 'Esc'
          default:
            return part.length === 1 ? part.toUpperCase() : part
        }
      })
      return output.join(isMac ? '' : '+')
    })
    .join(' then ')
}

export function useKeyboard() {
  installListener()

  function register(keys: string, handler: ShortcutHandler, description: string, scope: string) {
    const normalized = normalizeShortcut(keys)
    const next = new Map(shortcuts.value)
    next.set(normalized, { keys: normalized, handler, description, scope })
    shortcuts.value = next
  }

  function unregister(keys: string) {
    const normalized = normalizeShortcut(keys)
    const next = new Map(shortcuts.value)
    next.delete(normalized)
    shortcuts.value = next
  }

  const modLabel = computed(() => (isMac ? '⌘' : 'Ctrl'))

  return {
    register,
    unregister,
    shortcuts,
    isMac,
    modLabel,
    prettyShortcut,
  }
}
