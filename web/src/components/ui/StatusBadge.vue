<script setup lang="ts">
import { computed } from 'vue'
import { CircleAlert, CircleCheck, Clock, Hourglass, LoaderCircle } from '@lucide/vue'
import { badgeInfo } from '@/utils/status'

const props = withDefaults(defineProps<{ status: string; suffix?: string; size?: 'sm' | 'md' }>(), {
  suffix: '',
  size: 'md',
})

const info = computed(() => badgeInfo(props.status))
const icon = computed(() => {
  switch (info.value.tone) {
    case 'saved':
      return CircleCheck
    case 'downloading':
      return LoaderCircle
    case 'failed':
      return CircleAlert
    case 'waiting':
      return Hourglass
    default:
      return Clock
  }
})
</script>

<template>
  <span class="badge" :class="[`tone-${info.tone}`, `s-${size}`]">
    <component :is="icon" :class="{ spin: info.tone === 'downloading' }" aria-hidden="true" />
    <span class="badge-text">{{ info.text }}{{ suffix }}</span>
  </span>
</template>

<style scoped>
.badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 24px;
  padding: 0 9px 0 7px;
  border-radius: 999px;
  border: 1px solid var(--gray-border);
  background: var(--gray-soft);
  color: var(--gray);
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
  transition:
    background var(--t-med) var(--ease-out),
    color var(--t-med) var(--ease-out),
    border-color var(--t-med) var(--ease-out);
}
.badge svg {
  width: 13px;
  height: 13px;
  flex: none;
}
.s-sm {
  height: 21px;
  font-size: 11px;
  padding: 0 7px 0 6px;
}
.s-sm svg {
  width: 11px;
  height: 11px;
}
.tone-saved {
  color: var(--green);
  background: var(--green-soft);
  border-color: var(--green-border);
}
.tone-downloading {
  color: var(--blue);
  background: var(--blue-soft);
  border-color: var(--blue-border);
}
.tone-failed {
  color: var(--red);
  background: var(--red-soft);
  border-color: var(--red-border);
}
.tone-waiting {
  color: var(--amber);
  background: var(--amber-soft);
  border-color: var(--amber-border);
}
</style>
