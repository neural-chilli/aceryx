function isAbsoluteURL(value: string): boolean {
  return /^https?:\/\//i.test(value) || value.startsWith('data:') || value.startsWith('blob:')
}

function normalizePath(path: string): string {
  if (!path) {
    return '/'
  }
  return path.startsWith('/') ? path : `/${path}`
}

export function backendHTTPURL(path: string): string {
  if (!path) {
    return ''
  }
  if (isAbsoluteURL(path)) {
    return path
  }
  if (path.startsWith('//')) {
    return `${window.location.protocol}${path}`
  }
  if (import.meta.env.DEV) {
    return `http://localhost:8080${normalizePath(path)}`
  }
  return normalizePath(path)
}

export function backendWSURL(path: string, params: URLSearchParams): string {
  const normalizedPath = normalizePath(path)
  if (import.meta.env.DEV) {
    const query = params.toString()
    return `ws://localhost:8080${normalizedPath}${query ? `?${query}` : ''}`
  }

  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const query = params.toString()
  return `${protocol}://${window.location.host}${normalizedPath}${query ? `?${query}` : ''}`
}
