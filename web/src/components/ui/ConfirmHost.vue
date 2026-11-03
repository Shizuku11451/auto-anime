<script setup lang="ts">
import BaseDialog from './BaseDialog.vue'
import BaseButton from './BaseButton.vue'
import { useConfirm } from '@/stores/confirm'

const { state, resolveConfirm } = useConfirm()
</script>

<template>
  <BaseDialog :open="state.open" :title="state.title" size="sm" @close="resolveConfirm(false)">
    <p class="confirm-text">{{ state.text }}</p>
    <label v-if="state.checkLabel" class="confirm-check"><input v-model="state.checked" type="checkbox" />{{ state.checkLabel }}</label>
    <template #footer>
      <BaseButton autofocus @click="resolveConfirm(false)">{{ state.cancelText }}</BaseButton>
      <BaseButton :variant="state.danger ? 'danger' : 'primary'" @click="resolveConfirm(true)">
        {{ state.okText }}
      </BaseButton>
    </template>
  </BaseDialog>
</template>

<style scoped>
.confirm-text {
  color: var(--text-2);
  line-height: 1.7;
  padding: 4px 0 14px;
  font-size: 14px;
}
.confirm-check { display: flex; align-items: flex-start; gap: 8px; padding-bottom: 14px; font-size: 14px; }
.confirm-check input { flex: none; margin-top: 3px; }
</style>
