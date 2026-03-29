import { ref } from 'vue'
import type { Theme } from '../types'

const currentTheme = ref<Theme | null>(null)

function clearThemeOverrides(previous: Theme | null) {
  if (!previous) {
    return
  }
  const root = document.documentElement
  for (const key of Object.keys(previous.overrides ?? {})) {
    root.style.removeProperty(key)
  }
}

export function useTheme() {
  const apply = (theme: Theme) => {
    const root = document.documentElement
    clearThemeOverrides(currentTheme.value)
    root.classList.remove('p-dark')
    if (theme.mode === 'dark') {
      root.classList.add('p-dark')
    }
    for (const [prop, value] of Object.entries(theme.overrides ?? {})) {
      root.style.setProperty(prop, value)
    }
    currentTheme.value = theme
  }

  return { currentTheme, apply }
}
