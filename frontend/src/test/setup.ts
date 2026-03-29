import { afterEach } from 'vitest'

afterEach(() => {
  document.documentElement.className = ''
  document.documentElement.removeAttribute('style')
  document.body.innerHTML = ''
  sessionStorage.clear()
})
