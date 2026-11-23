import { ref } from 'vue'

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
