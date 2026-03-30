<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import Select from 'primevue/select'
import Button from 'primevue/button'
import { useAuth } from '../composables/useAuth'
import { useTerminology } from '../composables/useTerminology'
import { useWebSocket } from '../composables/useWebSocket'

type FeedEvent = {
  id: string
  type: string
  text: string
  icon: string
  case_id: string
  case_number: string
  actor_name: string
  timestamp: string
  metadata?: Record<string, unknown>
}

const { authFetch } = useAuth()
const { t } = useTerminology()
const { messages, open } = useWebSocket()

const feed = ref<FeedEvent[]>([])
const loading = ref(false)
const hasMore = ref(true)
const filter = ref<'all' | 'cases' | 'tasks' | 'ai' | 'documents' | 'sla'>('all')
const timelineEl = ref<HTMLElement | null>(null)
const hasUnreadTop = ref(false)

const filteredFeed = computed(() => {
  if (filter.value === 'all') {
    return feed.value
  }
  return feed.value.filter((item) => {
    if (filter.value === 'cases') return item.type.startsWith('case.')
    if (filter.value === 'tasks') return item.type.startsWith('task.')
    if (filter.value === 'ai') return item.type.startsWith('agent.')
    if (filter.value === 'documents') return item.type.startsWith('document.')
    if (filter.value === 'sla') return item.type === 'system.sla_breach'
    return true
  })
})

function iconEmoji(type: string): string {
  if (type.startsWith('case.')) return '📁'
  if (type.startsWith('task.')) return '✅'
  if (type.startsWith('agent.')) return '🤖'
  if (type.startsWith('document.')) return '📄'
  if (type.startsWith('system.')) return '⚠️'
  return '•'
}

function relativeTime(ts: string): string {
  const date = new Date(ts)
  const delta = Math.max(0, Date.now() - date.getTime())
  const mins = Math.floor(delta / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins} minute${mins === 1 ? '' : 's'} ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'} ago`
  const days = Math.floor(hours / 24)
  return `${days} day${days === 1 ? '' : 's'} ago`
}

function queryParamsForNextPage(): URLSearchParams {
  const q = new URLSearchParams()
  q.set('limit', '50')
  q.set('filter', filter.value)
  if (feed.value.length > 0) {
    const last = feed.value[feed.value.length - 1]
    q.set('before_time', new Date(last.timestamp).toISOString())
    q.set('before_id', last.id)
  }
  return q
}

async function loadMore(reset = false) {
  if (loading.value) return
  if (!hasMore.value && !reset) return

  if (reset) {
    feed.value = []
    hasMore.value = true
  }

  loading.value = true
  try {
    const res = await authFetch(`/activity?${queryParamsForNextPage().toString()}`)
    if (!res.ok) return
    const batch = (await res.json()) as FeedEvent[]
    if (reset) {
      feed.value = batch
    } else {
      const seen = new Set(feed.value.map((item) => item.id))
      const merged = [...feed.value]
      for (const item of batch) {
        if (!seen.has(item.id)) {
          merged.push(item)
        }
      }
      feed.value = merged
    }
    if (batch.length < 50) {
      hasMore.value = false
    }
  } finally {
    loading.value = false
  }
}

function onScroll() {
  const el = timelineEl.value
  if (!el) return
  const nearBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 20
  if (nearBottom) {
    void loadMore()
  }
}

function isNearTop(): boolean {
  const el = timelineEl.value
  if (!el) return true
  return el.scrollTop < 40
}

function scrollToTop() {
  timelineEl.value?.scrollTo({ top: 0, behavior: 'smooth' })
  hasUnreadTop.value = false
}

onMounted(() => {
  void loadMore(true)
  open()
})

watch(filter, async () => {
  await loadMore(true)
})

watch(messages, (all) => {
  const last = all[all.length - 1]
  if (!last || last.type !== 'activity' || !last.payload) {
    return
  }
  const payload = last.payload as FeedEvent
  if (payload.case_id && payload.id) {
    feed.value = [payload, ...feed.value.filter((item) => item.id !== payload.id)]
  }
  if (!isNearTop()) {
    hasUnreadTop.value = true
  }
})

onUnmounted(() => {
  hasUnreadTop.value = false
})
</script>

<template>
  <section class="activity">
    <div class="header">
      <h1>{{ t('Activity') }}</h1>
      <Select
        v-model="filter"
        :options="['all', 'cases', 'tasks', 'ai', 'documents', 'sla']"
        class="filter"
      />
    </div>

    <Button
      v-if="hasUnreadTop"
      class="new-pill"
      label="New activity"
      size="small"
      @click="scrollToTop"
    />

    <div ref="timelineEl" class="timeline" @scroll="onScroll">
      <TransitionGroup name="slide" tag="div" class="items">
        <article v-for="item in filteredFeed" :key="item.id" class="item">
          <div class="icon" :title="item.type">{{ iconEmoji(item.type) }}</div>
          <div class="content">
            <p class="text">{{ item.text }}</p>
            <div class="meta">
              <RouterLink :to="`/cases/${item.case_id}`" class="case-link">{{ item.case_number }}</RouterLink>
              <span>{{ relativeTime(item.timestamp) }}</span>
            </div>
          </div>
        </article>
      </TransitionGroup>
      <p v-if="!loading && filteredFeed.length === 0" class="empty">No {{ t('Activity') }} yet.</p>
      <p v-if="loading" class="loading">Loading...</p>
    </div>
  </section>
</template>

<style scoped>
.activity {
  display: grid;
  gap: 0.75rem;
  height: calc(100vh - 8rem);
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
}

.header h1 {
  margin: 0;
}

.filter {
  min-width: 10rem;
}

.new-pill {
  width: fit-content;
}

.timeline {
  overflow: auto;
  padding-right: 0.3rem;
}

.items {
  display: grid;
  gap: 0.6rem;
}

.item {
  display: grid;
  grid-template-columns: 1.5rem 1fr;
  gap: 0.6rem;
  padding: 0.55rem 0.65rem;
  border: 1px solid #dbe3ef;
  border-radius: 0.6rem;
  background: #fff;
}

.icon {
  font-size: 1.05rem;
}

.text {
  margin: 0 0 0.2rem;
}

.meta {
  display: inline-flex;
  gap: 0.5rem;
  font-size: 0.82rem;
  color: #64748b;
}

.case-link {
  color: var(--acx-brand-primary, #2563eb);
  text-decoration: none;
}

.slide-enter-active {
  transition: all 0.22s ease;
}

.slide-enter-from {
  opacity: 0;
  transform: translateY(-8px);
}

.empty,
.loading {
  color: #64748b;
}

@media (max-width: 640px) {
  .activity {
    height: calc(100vh - 10.5rem);
    gap: 0.55rem;
  }

  .header {
    gap: 0.5rem;
  }

  .filter {
    min-width: 8rem;
  }

  .item {
    grid-template-columns: 1.15rem 1fr;
    gap: 0.5rem;
    padding: 0.45rem 0.5rem;
  }

  .icon {
    font-size: 0.9rem;
  }

  .meta {
    font-size: 0.74rem;
    gap: 0.35rem;
  }
}
</style>
