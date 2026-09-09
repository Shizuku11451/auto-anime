<script setup lang="ts">
import { computed, toRef } from 'vue'
import { TransitionPresets, useTransition } from '@vueuse/core'

const props = withDefaults(defineProps<{ value: number; duration?: number }>(), { duration: 700 })

const source = toRef(props, 'value')
const animated = useTransition(source, {
  duration: props.duration,
  transition: TransitionPresets.easeOutExpo,
})
const display = computed(() => Math.round(animated.value).toLocaleString('zh-CN'))
</script>

<template>
  <span class="num">{{ display }}</span>
</template>

<style scoped>
.num {
  font-variant-numeric: tabular-nums;
}
</style>
