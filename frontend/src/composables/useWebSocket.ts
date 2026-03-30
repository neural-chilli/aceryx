import { computed, onBeforeUnmount, ref } from 'vue'
import { useAuth } from './useAuth'

const messages = ref<any[]>([])
let socket: WebSocket | null = null
let reconnectTimer: number | null = null
let retries = 0

function connect() {
  const { token } = useAuth()
  if (!token.value) {
    return
  }

  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const url = `${protocol}://${window.location.host}/ws?token=${encodeURIComponent(token.value)}`
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
  socket.onclose = () => {
    scheduleReconnect()
  }
  socket.onerror = () => {
    socket?.close()
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
    if (!socket || socket.readyState === WebSocket.CLOSED) {
      connect()
    }
  }
  const close = () => {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    socket?.close()
    socket = null
  }

  onBeforeUnmount(() => {
    close()
  })

  const activityMessages = computed(() => messages.value.filter((msg) => msg?.type === 'activity'))

  return { messages, activityMessages, open, close }
}
