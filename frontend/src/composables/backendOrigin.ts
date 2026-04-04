function isAbsoluteURL(value: string): boolean {
  return /^https?:\/\//i.test(value) || value.startsWith('data:') || value.startsWith('blob:')
}

function normalizePath(path: string): string {
  if (!path) {
    return '/'
  }
  const trimmed = path.replace(/^\/+/, '')
  return `/${trimmed}`
}

export function backendHTTPURL(path: string): string {
  if (!path) {
    return ''
  }
  if (isAbsoluteURL(path)) {
    return path
  }
  if (import.meta.env.DEV) {
    return `http://localhost:8080${normalizePath(path)}`
  }
  return normalizePath(path)
}

export function backendWSURL(path: string, params: URLSearchParams): string {
  const [rawPath, existingQuery = ''] = normalizePath(path).split('?', 2)
  const normalizedPath = rawPath || '/'
  const mergedParams = new URLSearchParams(existingQuery)
  params.forEach((value, key) => {
    mergedParams.set(key, value)
  })
  const query = mergedParams.toString()

  if (import.meta.env.DEV) {
    return `ws://localhost:8080${normalizedPath}${query ? `?${query}` : ''}`
  }

  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  return `${protocol}://${window.location.host}${normalizedPath}${query ? `?${query}` : ''}`
}
