import { computed, ref } from 'vue'
import { http } from '@/api/client'
export type Theme = 'light' | 'dark'

const THEME_KEY = 'theme'
const META_COLOR: Record<Theme, string> = { light: '#f6f8f6', dark: '#111615' }

const theme = ref<Theme>(document.documentElement.dataset.theme === 'light' ? 'light' : 'dark')

/* 桌面窗口的原生标题栏跟随应用主题；服务模式、非 Windows 或窗口尚未创建时后端返回
   applied=false，前端忽略结果即可。 */
function syncDesktopTheme(next: Theme) {
  void http.post('/api/desktop/theme', { theme: next }).catch(() => {})
}

// 页面加载时先同步一次：存储的主题与系统主题不一致时标题栏也能匹配。
syncDesktopTheme(theme.value)

function storedTheme(): Theme | null {
  try {
    const t = localStorage.getItem(THEME_KEY)
    return t === 'light' || t === 'dark' ? t : null
  } catch {
    return null
  }
}

function applyTheme(next: Theme) {
  theme.value = next
  document.documentElement.dataset.theme = next
  const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
  if (meta) meta.content = META_COLOR[next]
  syncDesktopTheme(next)
}

/* 未手动选择过主题时跟随系统亮暗变化 */
const media = window.matchMedia ? window.matchMedia('(prefers-color-scheme: light)') : null
media?.addEventListener?.('change', (e) => {
  if (!storedTheme()) applyTheme(e.matches ? 'light' : 'dark')
})

type ViewTransitionDocument = Document & {
  startViewTransition?: (update: () => void) => { ready: Promise<void> }
}

export function useTheme() {
  const isLight = computed(() => theme.value === 'light')

  function set(next: Theme) {
    applyTheme(next)
    try {
      localStorage.setItem(THEME_KEY, next)
    } catch {
      /* 隐私模式下忽略 */
    }
  }

  /** 切换主题；支持 View Transitions 的浏览器会从点击位置做圆形揭开动画 */
  function toggle(event?: MouseEvent) {
    const next: Theme = theme.value === 'light' ? 'dark' : 'light'
    const doc = document as ViewTransitionDocument
    const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    if (!doc.startViewTransition || reduce) {
      set(next)
      return
    }
    const x = event?.clientX ?? window.innerWidth / 2
    const y = event?.clientY ?? window.innerHeight / 2
    const radius = Math.hypot(Math.max(x, window.innerWidth - x), Math.max(y, window.innerHeight - y))
    const transition = doc.startViewTransition(() => set(next))
    transition.ready
      .then(() => {
        document.documentElement.animate(
          { clipPath: [`circle(0px at ${x}px ${y}px)`, `circle(${radius}px at ${x}px ${y}px)`] },
          { duration: 220, easing: 'cubic-bezier(0.22, 1, 0.36, 1)', pseudoElement: '::view-transition-new(root)' },
        )
      })
      .catch(() => {
        /* 动画不可用时主题已经切换，无需处理 */
      })
  }

  return { theme, isLight, set, toggle }
}
