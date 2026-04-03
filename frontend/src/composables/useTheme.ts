import { ref } from 'vue'
import type { Theme } from '../types'

const THEME_KEY = 'acx:theme-id'
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
    try {
      localStorage.setItem(THEME_KEY, theme.id)
    } catch {
      /* storage unavailable */
    }
  }

  const savedThemeID = (): string | null => {
    try {
      return localStorage.getItem(THEME_KEY)
    } catch {
      return null
    }
  }

  return { currentTheme, apply, savedThemeID }
}
