import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const width = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
let installed = false

function onResize() {
  width.value = window.innerWidth
}

function ensureInstalled() {
  if (installed || typeof window === 'undefined') {
    return
  }
  installed = true
  window.addEventListener('resize', onResize)
}

export function useBreakpoint() {
  ensureInstalled()

  onMounted(() => {
    if (typeof window !== 'undefined') {
      width.value = window.innerWidth
    }
  })

  onBeforeUnmount(() => {
    // keep global listener for shared reactive width
  })

  const isSm = computed(() => width.value < 640)
  const isMd = computed(() => width.value >= 640 && width.value <= 1024)
  const isMobileOrTablet = computed(() => width.value <= 1024)
  const isDesktop = computed(() => width.value > 1024)

  return {
    width,
    isSm,
    isMd,
    isMobileOrTablet,
    isDesktop,
  }
}
