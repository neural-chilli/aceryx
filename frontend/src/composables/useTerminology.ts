import { storeToRefs } from 'pinia'
import { useTerminologyStore } from '../stores/terminology'

export function useTerminology() {
  const store = useTerminologyStore()
  const { terms } = storeToRefs(store)

  const t = (key: string): string => terms.value[key] ?? key

  return { terms, t, setTerms: store.setTerms }
}
