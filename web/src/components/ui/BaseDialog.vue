<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { onKeyStroke } from '@vueuse/core'
import { X } from '@lucide/vue'
import BaseButton from './BaseButton.vue'
import { isTopDialog, popDialog, pushDialog } from '@/composables/dialogStack'

const props = withDefaults(
  defineProps<{
    open: boolean
    title?: string
    size?: 'sm' | 'md' | 'lg' | 'xl'
    /** 是否显示右上角关闭按钮并允许 Esc 关闭 */
    closable?: boolean
    /** 点击遮罩不关闭 */
    persistent?: boolean
  }>(),
  { size: 'md', closable: true, persistent: false },
)

const emit = defineEmits<{ close: [] }>()

const panel = ref<HTMLElement | null>(null)
let returnFocus: HTMLElement | null = null

function trapFocus(e: KeyboardEvent) {
  if (e.key !== 'Tab' || !isTopDialog(me)) return
  const controls = Array.from(panel.value?.querySelectorAll<HTMLElement>('button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]') ?? []).filter(el => el.getClientRects().length > 0)
  const first = controls[0]
  const last = controls.at(-1)
  if (!first) { e.preventDefault(); panel.value?.focus(); return }
  if (e.shiftKey && (document.activeElement === first || document.activeElement === panel.value)) { e.preventDefault(); last?.focus() }
  else if (!e.shiftKey && (document.activeElement === last || document.activeElement === panel.value)) { e.preventDefault(); first.focus() }
}

const me = Symbol('dialog')
let registered = false
function register(on: boolean) {
  if (on === registered) return
  registered = on
  if (on) pushDialog(me)
  else popDialog(me)
}

watch(
  () => props.open,
  async (open) => {
    if (open) returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const wasTop = isTopDialog(me)
    register(open)
    if (!open) { if (wasTop) returnFocus?.focus({ preventScroll: true }); return }
    await nextTick()
    const target =
      panel.value?.querySelector<HTMLElement>('[autofocus]') ??
      panel.value?.querySelector<HTMLElement>('input:not([type="hidden"]):not([disabled]), select, textarea')
    if (props.open && isTopDialog(me)) (target ?? panel.value)?.focus({ preventScroll: true })
  },
  { immediate: true },
)

onBeforeUnmount(() => { const wasTop = isTopDialog(me); register(false); if (wasTop) returnFocus?.focus({ preventScroll: true }) })

onKeyStroke('Escape', (e) => {
  if (!props.open || !props.closable) return
  if (!isTopDialog(me)) return
  e.preventDefault()
  emit('close')
})

function onBackdrop(e: MouseEvent) {
  if (e.target === e.currentTarget && props.closable && !props.persistent && isTopDialog(me)) emit('close')
}
</script>

<template>
  <Teleport to="body">
    <Transition name="dlg">
      <div v-if="open" class="overlay" role="presentation" @mousedown.self="onBackdrop">
        <div ref="panel" class="dialog" :class="`size-${size}`" role="dialog" aria-modal="true" :aria-label="title" tabindex="-1" @keydown="trapFocus">
          <header v-if="title || $slots.header || closable" class="dialog-head">
            <div class="dialog-head-main">
              <slot name="header">
                <h2 class="dialog-title">{{ title }}</h2>
              </slot>
            </div>
            <BaseButton v-if="closable" variant="ghost" size="sm" icon-only :icon="X" aria-label="关闭" @click="emit('close')" />
          </header>
          <div class="dialog-body">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="dialog-foot">
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: grid;
  place-items: center;
  padding: 24px;
  background: var(--overlay);
  backdrop-filter: blur(4px);
  -webkit-backdrop-filter: blur(4px);
}

.dialog {
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: 560px;
  max-height: min(88dvh, 920px);
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: var(--r-xl);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  will-change: transform, opacity;
}
.size-sm {
  max-width: 440px;
}
.size-lg {
  max-width: 760px;
}
.size-xl {
  max-width: 960px;
}

.dialog-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 20px 18px 18px 24px;
  border-bottom: 1px solid var(--line);
}
.dialog-head-main {
  flex: 1;
  min-width: 0;
}
.dialog-title {
  font-size: 17px;
  font-weight: 700;
  letter-spacing: -0.01em;
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.dialog-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 20px 24px;
  overscroll-behavior: contain;
}
.dialog-foot {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  padding: 14px 24px 20px;
  border-top: 1px solid var(--line);
  background: var(--surface);
}
.dialog-foot :deep(.spacer) {
  flex: 1;
}

/* 入场：遮罩淡入 + 面板弹性上浮；离场：快速缩回 */
.dlg-enter-active {
  transition: opacity var(--t-med) var(--ease-out);
}
.dlg-enter-active .dialog {
  transition:
    transform var(--t-med) var(--ease-out),
    opacity var(--t-med) var(--ease-out);
}
.dlg-leave-active {
  transition: opacity 170ms var(--ease-in-out);
}
.dlg-leave-active .dialog {
  transition:
    transform 170ms var(--ease-in-out),
    opacity 170ms var(--ease-in-out);
}
.dlg-enter-from {
  opacity: 0;
}
.dlg-enter-from .dialog {
  opacity: 0;
  transform: translateY(8px) scale(0.99);
}
.dlg-leave-to {
  opacity: 0;
}
.dlg-leave-to .dialog {
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}

/* 手机：底部抽屉 */
@media (max-width: 640px) {
  .overlay {
    place-items: end center;
    padding: 0;
  }
  .dialog {
    max-width: none;
    max-height: 92dvh;
    border-radius: var(--r-xl) var(--r-xl) 0 0;
    border-bottom: 0;
  }
  .dialog-head {
    padding: 16px 12px 10px 18px;
  }
  .dialog-body {
    padding: 18px;
  }
  .dialog-foot {
    padding: 12px 18px calc(16px + env(safe-area-inset-bottom));
    flex-wrap: wrap;
  }
  .dlg-enter-from .dialog {
    transform: translateY(40px);
  }
  .dlg-leave-to .dialog {
    transform: translateY(24px);
  }
}
</style>
