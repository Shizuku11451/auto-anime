/* 统一的请求封装：JSON 编解码、错误文本提取、会话过期只跳转一次。 */

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

interface RequestOptions extends Omit<RequestInit, 'body'> {
  /** 自动 JSON.stringify 并设置 Content-Type */
  json?: unknown
  /** 原始请求体（文本、FormData 等） */
  body?: BodyInit | null
}

let redirectingToLogin = false

/** 当前地址（含 hash 路由），登录后原路返回 */
function currentLocation(): string {
  return location.pathname + location.search + location.hash
}

export async function api<T = unknown>(url: string, options: RequestOptions = {}): Promise<T> {
  const { json, headers, ...rest } = options
  const init: RequestInit = { ...rest, headers: new Headers(headers) }
  if (json !== undefined) {
    ;(init.headers as Headers).set('Content-Type', 'application/json')
    init.body = JSON.stringify(json)
  }
  const res = await fetch(url, init)
  if (res.status === 401) {
    if (!redirectingToLogin) {
      redirectingToLogin = true
      location.href = '/login?next=' + encodeURIComponent(currentLocation())
    }
    throw new ApiError('登录已过期，正在跳转到登录页', 401)
  }
  if (!res.ok) {
    let detail = ''
    try {
      detail = (await res.text()).trim()
      try {
        const parsed = JSON.parse(detail) as { detail?: string; error?: string }
        detail = parsed.detail || parsed.error || detail
      } catch {
        /* 纯文本错误 */
      }
    } catch {
      /* 忽略读取失败 */
    }
    throw new ApiError(detail || `请求失败 HTTP ${res.status}`, res.status)
  }
  const type = res.headers.get('Content-Type') || ''
  if (type.includes('json')) return (await res.json()) as T
  return (await res.text()) as unknown as T
}

export const http = {
  get: <T>(url: string) => api<T>(url),
  post: <T>(url: string, json?: unknown) => api<T>(url, { method: 'POST', json }),
  put: <T>(url: string, json?: unknown) => api<T>(url, { method: 'PUT', json }),
  patch: <T>(url: string, json?: unknown) => api<T>(url, { method: 'PATCH', json }),
  delete: <T>(url: string) => api<T>(url, { method: 'DELETE' }),
}

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err ?? '未知错误')
}
