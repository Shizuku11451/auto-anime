<script setup lang="ts">
import { computed, type Component } from 'vue'
import { Save, Undo2 } from '@lucide/vue'
import BaseButton from '@/components/ui/BaseButton.vue'
import { isEditableSection, useSettings, type SectionId } from '@/composables/useSettingsForm'

const props = defineProps<{ id: SectionId; title: string; icon: Component; description?: string }>()
const s = useSettings()
const editable = computed(() => isEditableSection(props.id))
const dirty = computed(() => s.isDirty(props.id))
const locked = computed(() => editable.value && (!s.ready.value || s.saving.value))
const status = computed(() => {
  if (s.saving.value) return '正在保存…'
  if (s.loading.value) return '正在读取设置…'
  if (s.loadErr.value) return '读取失败，暂不能保存'
  return dirty.value ? '有未保存的更改' : '已保存'
})
</script>

<template>
  <section :id="`settings-${id}`" class="group card" :aria-labelledby="`settings-${id}-title`">
    <header class="group-head">
      <span class="group-ico"><component :is="icon" aria-hidden="true" /></span>
      <div>
        <h2 :id="`settings-${id}-title`">{{ title }}</h2>
        <p v-if="description">{{ description }}</p>
      </div>
    </header>

    <form v-if="editable" novalidate @submit.prevent="s.save(id)">
      <fieldset class="group-body" :class="{ loading: s.loading.value }" :disabled="locked" :aria-busy="s.loading.value">
        <slot />
      </fieldset>
      <footer class="save-bar" :class="{ dirty }">
        <div class="save-status" role="status" aria-live="polite">
          <strong>{{ status }}</strong>
        </div>
        <Transition name="fade">
          <p v-if="s.saveError.value" class="form-msg" role="alert">{{ s.saveError.value }}</p>
        </Transition>
        <div class="save-actions">
          <BaseButton v-if="dirty" variant="ghost" :icon="Undo2" :disabled="locked || s.confirming.value" @click="s.reset(id)">撤销</BaseButton>
          <BaseButton type="submit" variant="primary" :icon="Save" :loading="s.saving.value" :disabled="!s.ready.value || !dirty || s.confirming.value">保存</BaseButton>
        </div>
      </footer>
    </form>
    <div v-else class="group-body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.group {
  padding: 20px 22px 0;
  animation: rise-in var(--t-slow) var(--ease-out) both;
}
.group-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 18px;
}
.group-head h2 {
  font-size: 15.5px;
  font-weight: 700;
  letter-spacing: -0.01em;
}
.group-head p {
  margin-top: 3px;
  color: var(--text-3);
  font-size: 12.5px;
  line-height: 1.55;
}
.group-ico {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 11px;
  color: var(--accent);
  background: var(--accent-soft);
  flex: none;
}
.group-ico :deep(svg) {
  width: 17px;
  height: 17px;
}
.group-body {
  min-width: 0;
  margin: 0;
  padding: 0 0 12px;
  border: 0;
  transition: opacity var(--t-med) var(--ease-out);
}
.group-body.loading {
  opacity: 0.55;
  pointer-events: none;
}
.save-bar {
  position: sticky;
  bottom: 0;
  z-index: 2;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 14px;
  margin: 0 -22px;
  padding: 14px 22px;
  border-top: 1px solid var(--line);
  border-radius: 0 0 var(--r-lg) var(--r-lg);
  background: var(--surface);
}
.save-bar.dirty { border-top-color: var(--accent-border); }
.save-status { flex: 1; min-width: 120px; color: var(--text-3); font-size: 13px; }
.save-bar.dirty .save-status { color: var(--accent); }
.save-status strong { font-weight: 550; }
.save-bar .form-msg { margin: 0; overflow-wrap: anywhere; }
.save-actions { display: flex; gap: 8px; }
@media (max-width: 900px) {
  .save-bar { bottom: calc(84px + env(safe-area-inset-bottom)); }
}
@media (max-width: 640px) {
  .group { padding: 16px 16px 0; }
  .save-bar { margin: 0 -16px; padding: 12px 16px; flex-direction: column; align-items: stretch; gap: 10px; }
  .save-actions > :deep(.btn) { flex: 1; }
}
</style>
