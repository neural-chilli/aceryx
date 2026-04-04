import { computed, ref } from 'vue'
import { useAuth } from './useAuth'
import { backendWSURL } from './backendOrigin'

const messages = ref<any[]>([])
let socket: WebSocket | null = null
let reconnectTimer: number | null = null
let heartbeatTimer: number | null = null
let lastMessageAt = 0
let retries = 0
let keepAlive = false
let intentionalClose = false
const heartbeatIntervalMs = 30_000
const staleThresholdMs = 90_000

function stopHeartbeat() {
  if (heartbeatTimer !== null) {
    clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

function startHeartbeat() {
  stopHeartbeat()
  lastMessageAt = Date.now()
  heartbeatTimer = window.setInterval(() => {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return
    }
    const now = Date.now()
    if (now - lastMessageAt > staleThresholdMs) {
      socket.close(4000, 'stale websocket')
      return
    }
    socket.send(JSON.stringify({ type: 'ping', at: new Date(now).toISOString() }))
  }, heartbeatIntervalMs)
}

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
    startHeartbeat()
  }
  socket.onmessage = (ev) => {
    lastMessageAt = Date.now()
    try {
      const payload = JSON.parse(ev.data)
      messages.value = [...messages.value.slice(-99), payload]
    } catch {
      // ignore malformed ws message
    }
  }
  socket.onclose = (event) => {
    stopHeartbeat()
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
    stopHeartbeat()
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
