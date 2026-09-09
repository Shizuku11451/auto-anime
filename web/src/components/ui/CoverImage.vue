<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import { Film } from '@lucide/vue'

const props = withDefaults(
  defineProps<{
    src?: string
    alt?: string
    /** CSS aspect-ratio 值，如 "2 / 3" */
    ratio?: string
    icon?: Component
    radius?: string
  }>(),
  { src: '', alt: '', ratio: '2 / 3', radius: 'var(--r-md)' },
)

/* 函数式图标组件不能作为 withDefaults 的默认值（会被当成工厂函数调用），这里用 computed 兜底 */
const iconComp = computed(() => props.icon ?? Film)

const loaded = ref(false)
const failed = ref(false)

watch(
  () => props.src,
  () => {
    loaded.value = false
    failed.value = false
  },
)
</script>

<template>
  <div class="cover" :style="{ aspectRatio: ratio, borderRadius: radius }" :class="{ 'has-img': src && !failed }">
    <img
      v-if="src && !failed"
      :src="src"
      :alt="alt"
      loading="lazy"
      decoding="async"
      :class="{ loaded }"
      @load="loaded = true"
      @error="failed = true"
    />
    <div v-if="!src || failed || !loaded" class="ph" :class="{ shimmer: src && !failed && !loaded }" aria-hidden="true">
      <component :is="iconComp" v-if="!src || failed" />
    </div>
    <slot />
  </div>
</template>

<style scoped>
.cover {
  position: relative;
  overflow: hidden;
  background: var(--surface-3);
  flex: none;
}
img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  opacity: 0;
  transform: scale(1.04);
  transition:
    opacity var(--t-slow) var(--ease-out),
    transform 700ms var(--ease-out);
}
img.loaded {
  opacity: 1;
  transform: none;
}
.ph {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  color: var(--text-3);
  background: linear-gradient(160deg, var(--surface-3), var(--surface-2));
}
.ph svg {
  width: 26%;
  height: 26%;
  max-width: 36px;
  max-height: 36px;
  opacity: 0.7;
}
.ph.shimmer {
  background: linear-gradient(100deg, var(--surface-3) 30%, var(--surface-2) 50%, var(--surface-3) 70%);
  background-size: 200% 100%;
  animation: shimmer 1.6s linear infinite;
}
</style>
