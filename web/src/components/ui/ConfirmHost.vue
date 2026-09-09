<script setup lang="ts">
import BaseDialog from './BaseDialog.vue'
import BaseButton from './BaseButton.vue'
import { useConfirm } from '@/stores/confirm'

const { state, resolveConfirm } = useConfirm()
</script>

<template>
  <BaseDialog :open="state.open" :title="state.title" size="sm" @close="resolveConfirm(false)">
    <p class="confirm-text">{{ state.text }}</p>
    <template #footer>
      <BaseButton @click="resolveConfirm(false)">{{ state.cancelText }}</BaseButton>
      <BaseButton :variant="state.danger ? 'danger' : 'primary'" autofocus @click="resolveConfirm(true)">
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
</style>
