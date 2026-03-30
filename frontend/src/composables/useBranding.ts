import type { Branding } from '../types'

let manifestBlobURL = ''

function clamp(v: number): number {
  return Math.max(0, Math.min(255, v))
}

function normalizeHex(color: string): string {
  const c = color.trim().replace('#', '')
  if (c.length === 3) {
    return `#${c[0]}${c[0]}${c[1]}${c[1]}${c[2]}${c[2]}`
  }
  if (c.length === 6) {
    return `#${c}`
  }
  return color
}

function adjust(hex: string, delta: number): string {
  const clean = normalizeHex(hex).replace('#', '')
  const r = clamp(parseInt(clean.slice(0, 2), 16) + delta)
  const g = clamp(parseInt(clean.slice(2, 4), 16) + delta)
  const b = clamp(parseInt(clean.slice(4, 6), 16) + delta)
  return `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${b.toString(16).padStart(2, '0')}`
}

export function useBranding() {
  const apply = (branding: Branding) => {
    if (branding.company_name) {
      document.title = branding.company_name
    }

    const root = document.documentElement
    const primary = branding.colors?.primary ?? '#2563eb'
    const secondary = branding.colors?.secondary ?? '#334155'
    const accent = branding.colors?.accent ?? '#0ea5e9'

    root.style.setProperty('--acx-brand-primary', primary)
    root.style.setProperty('--acx-brand-secondary', secondary)
    root.style.setProperty('--acx-brand-accent', accent)
    root.style.setProperty('--acx-brand-primary-dark', adjust(primary, -24))
    root.style.setProperty('--acx-brand-primary-light', adjust(primary, 24))

    root.style.setProperty('--p-primary-500', primary)
    root.style.setProperty('--p-primary-600', adjust(primary, -24))
    root.style.setProperty('--p-primary-400', adjust(primary, 24))

    const themeMeta = document.querySelector("meta[name='theme-color']") as HTMLMetaElement | null
    if (themeMeta) {
      themeMeta.content = primary
    }

    if (branding.favicon_url) {
      const link = document.querySelector("link[rel='icon']") as HTMLLinkElement | null
      if (link) {
        link.href = branding.favicon_url
      }
    }

    const manifestLink = document.querySelector("link[rel='manifest']") as HTMLLinkElement | null
    if (manifestLink) {
      if (manifestBlobURL && typeof URL.revokeObjectURL === 'function') {
        URL.revokeObjectURL(manifestBlobURL)
        manifestBlobURL = ''
      }
      const iconURL = branding.logo_url || '/logo-192.png'
      const manifest = {
        name: branding.company_name || 'Aceryx',
        short_name: branding.company_name || 'Aceryx',
        start_url: '/',
        display: 'standalone',
        background_color: primary,
        theme_color: primary,
        icons: [
          { src: iconURL, sizes: '192x192', type: 'image/png' },
          { src: iconURL, sizes: '512x512', type: 'image/png' },
        ],
      }
      if (typeof URL.createObjectURL === 'function') {
        manifestBlobURL = URL.createObjectURL(new Blob([JSON.stringify(manifest)], { type: 'application/manifest+json' }))
        manifestLink.href = manifestBlobURL
      }
    }
  }

  return { apply }
}
