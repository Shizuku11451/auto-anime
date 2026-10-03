<script setup lang="ts">
defineProps<{ title: string; description?: string; count?: number }>()
</script>

<template>
  <header class="page-head">
    <div class="page-head-text">
      <h1 class="page-title">
        {{ title }}
        <span v-if="count !== undefined" class="page-count">{{ count }}</span>
      </h1>
      <p v-if="description" class="page-desc">{{ description }}</p>
    </div>
    <div v-if="$slots.default" class="page-actions">
      <slot />
    </div>
  </header>
</template>

<style scoped>
.page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px 24px;
  flex-wrap: wrap;
  margin-bottom: 28px;
  padding-bottom: 24px;
  border-bottom: 1px solid var(--line);
  animation: rise-in var(--t-slow) var(--ease-out) both;
}
.page-head-text {
  min-width: 0;
}
.page-title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.2;
}
.page-count {
  display: inline-grid;
  place-items: center;
  min-width: 26px;
  height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-2);
  background: var(--surface-3);
  font-variant-numeric: tabular-nums;
}
.page-desc {
  margin-top: 8px;
  color: var(--text-2);
  font-size: 13.5px;
}
.page-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
@media (max-width: 640px) {
  .page-head {
    margin-bottom: 18px;
  }
  .page-title {
    font-size: 22px;
  }
  .page-actions {
    width: 100%;
  }
  .page-actions :deep(.btn),
  .page-actions :deep(.search) {
    flex: 1;
  }
}
</style>
