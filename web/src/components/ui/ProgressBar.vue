<script setup lang="ts">
import { computed } from 'vue'
import { clamp } from '@/utils/format'

const props = withDefaults(defineProps<{ value?: number; indeterminate?: boolean; height?: number }>(), {
  value: 0,
  indeterminate: false,
  height: 5,
})

const width = computed(() => (props.indeterminate ? 100 : clamp(props.value, 0, 100)))
</script>

<template>
  <div
    class="progress"
    :class="{ indeterminate }"
    :style="{ height: height + 'px' }"
    role="progressbar"
    :aria-valuenow="indeterminate ? undefined : Math.round(width)"
    aria-valuemin="0"
    aria-valuemax="100"
  >
    <div class="bar" :style="{ width: width + '%' }">
      <span class="glow" />
    </div>
  </div>
</template>

<style scoped>
.progress {
  position: relative;
  width: 100%;
  border-radius: 999px;
  background: var(--surface-3);
  overflow: hidden;
}
.bar {
  position: relative;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, var(--blue), var(--accent));
  transition: width 600ms var(--ease-out);
  overflow: hidden;
}
.glow {
  position: absolute;
  inset: 0;
  width: 40%;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.45), transparent);
  animation: bar-flow 1.8s var(--ease-in-out) infinite;
}
.indeterminate .bar {
  background: linear-gradient(90deg, transparent, var(--accent), transparent);
  width: 50% !important;
  animation: bar-flow 1.4s var(--ease-in-out) infinite;
}
.indeterminate .glow {
  display: none;
}
</style>
