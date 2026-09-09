import { ref } from 'vue'

export type ToastType = 'success' | 'error' | 'info'

export interface ToastMessage {
  id: number
  message: string
  type: ToastType
}

const toasts = ref<ToastMessage[]>([])
let seq = 0

export function dismissToast(id: number) {
  toasts.value = toasts.value.filter((t) => t.id !== id)
}

export function toast(message: string, type: ToastType = 'success', duration = 4200) {
  const id = ++seq
  toasts.value.push({ id, message, type })
  if (toasts.value.length > 5) toasts.value.shift()
  window.setTimeout(() => dismissToast(id), duration)
  return id
}

toast.success = (message: string) => toast(message, 'success')
toast.error = (message: string) => toast(message, 'error', 6000)
toast.info = (message: string) => toast(message, 'info')

export function useToasts() {
  return { toasts, toast, dismissToast }
}
