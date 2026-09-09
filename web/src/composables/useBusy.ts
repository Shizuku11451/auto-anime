import { ref } from 'vue'
import { errorMessage } from '@/api/client'
import { toast } from '@/stores/toast'

/** 包装异步操作：期间 busy=true，失败时以 toast 报错（或交给调用方处理）。 */
export function useBusy() {
  const busy = ref(false)

  async function run<T>(fn: () => Promise<T>, options: { silent?: boolean } = {}): Promise<T | undefined> {
    if (busy.value) return undefined
    busy.value = true
    try {
      return await fn()
    } catch (e) {
      if (!options.silent) toast.error(errorMessage(e))
      throw e
    } finally {
      busy.value = false
    }
  }

  return { busy, run }
}

/** 同一按钮组内按 id 追踪忙碌状态（列表行的下载/重试按钮） */
export function useBusySet() {
  const busy = ref(new Set<number | string>())

  function is(id: number | string) {
    return busy.value.has(id)
  }

  async function run<T>(id: number | string, fn: () => Promise<T>): Promise<T | undefined> {
    if (busy.value.has(id)) return undefined
    busy.value = new Set(busy.value).add(id)
    try {
      return await fn()
    } finally {
      const next = new Set(busy.value)
      next.delete(id)
      busy.value = next
    }
  }

  return { is, run }
}
