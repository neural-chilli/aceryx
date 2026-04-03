import { computed, ref } from 'vue'
import { useAuth } from './useAuth'
import { backendWSURL } from './backendOrigin'

const messages = ref<any[]>([])
let socket: WebSocket | null = null
let reconnectTimer: number | null = null
let retries = 0
let keepAlive = false
let intentionalClose = false

function connect() {
  if (socket && (socket.readyState === WebSocket.CONNECTING || socket.readyState === WebSocket.OPEN)) {
    return
  }

  const { token } = useAuth()
  if (!token.value) {
    return
  }

  intentionalClose = false
  const params = new URLSearchParams({ token: token.value })
  const url = backendWSURL('/ws', params)
  socket = new WebSocket(url)

  socket.onopen = () => {
    retries = 0
  }
  socket.onmessage = (ev) => {
    try {
      const payload = JSON.parse(ev.data)
      messages.value = [...messages.value.slice(-99), payload]
    } catch {
      // ignore malformed ws message
    }
  }
  socket.onclose = (event) => {
    socket = null
    if (intentionalClose || !keepAlive) {
      return
    }
    if (event.code === 1008 || event.code === 4401 || event.code === 4403) {
      return
    }
    scheduleReconnect()
  }
  socket.onerror = () => {
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.close()
    }
  }
}

function scheduleReconnect() {
  if (reconnectTimer !== null) {
    return
  }
  const delay = Math.min(30_000, Math.max(500, Math.pow(2, retries) * 500))
  retries += 1
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null
    connect()
  }, delay)
}

export function useWebSocket() {
  const open = () => {
    keepAlive = true
    connect()
  }
  const close = () => {
    keepAlive = false
    intentionalClose = true
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
      socket.close(1000, 'client close')
    }
    socket = null
  }

  const activityMessages = computed(() => messages.value.filter((msg) => msg?.type === 'activity'))

  return { messages, activityMessages, open, close }
}
