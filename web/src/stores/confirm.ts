import { reactive } from 'vue'

export interface ConfirmOptions {
  /** 确认按钮文字 */
  okText?: string
  /** 取消按钮文字 */
  cancelText?: string
  /** 危险操作：确认按钮使用红色 */
  danger?: boolean
}

interface ConfirmState {
  open: boolean
  title: string
  text: string
  okText: string
  cancelText: string
  danger: boolean
}

const state = reactive<ConfirmState>({
  open: false,
  title: '',
  text: '',
  okText: '确定',
  cancelText: '取消',
  danger: false,
})

let resolver: ((ok: boolean) => void) | null = null

/** 以 Promise 形式弹出确认框，关闭/取消都视为 false */
export function confirmDialog(title: string, text: string, options: ConfirmOptions = {}): Promise<boolean> {
  resolver?.(false)
  state.title = title
  state.text = text
  state.okText = options.okText ?? '确定'
  state.cancelText = options.cancelText ?? '取消'
  state.danger = !!options.danger
  state.open = true
  return new Promise<boolean>((resolve) => {
    resolver = resolve
  })
}

export function resolveConfirm(ok: boolean) {
  state.open = false
  const r = resolver
  resolver = null
  r?.(ok)
}

export function useConfirm() {
  return { state, confirmDialog, resolveConfirm }
}
