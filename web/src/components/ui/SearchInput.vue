<script setup lang="ts">
import { ref } from 'vue'
import { Search, X } from '@lucide/vue'

withDefaults(defineProps<{ placeholder?: string; width?: string }>(), { placeholder: '搜索', width: '280px' })
const model = defineModel<string>({ default: '' })
const emit = defineEmits<{ enter: [] }>()
const input = ref<HTMLInputElement | null>(null)
function clear() { model.value = ''; input.value?.focus() }
defineExpose({ focus: () => input.value?.focus() })
</script>

<template>
  <label class="search" :style="{ width }">
    <Search class="search-ic" aria-hidden="true" />
    <input ref="input" v-model="model" type="search" :aria-label="placeholder" :placeholder="placeholder" autocomplete="off" @keydown.enter="emit('enter')" />
    <Transition name="fade">
      <button v-if="model" type="button" class="clear" aria-label="清除搜索" @click="clear">
        <X />
      </button>
    </Transition>
  </label>
</template>

<style scoped>
.search {
  position: relative;
  display: block;
  max-width: 100%;
}
.search-ic {
  position: absolute;
  left: 12px;
  top: 50%;
  translate: 0 -50%;
  width: 15px;
  height: 15px;
  color: var(--text-3);
  pointer-events: none;
  transition: color var(--t-fast) var(--ease-out);
}
.search:focus-within .search-ic {
  color: var(--accent);
}
input {
  padding-left: 36px;
  padding-right: 34px;
  border-radius: var(--r-sm);
}
.clear {
  position: absolute;
  right: 6px;
  top: 50%;
  translate: 0 -50%;
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: 50%;
  background: transparent;
  color: var(--text-3);
  display: grid;
  place-items: center;
  cursor: pointer;
  transition:
    background var(--t-fast) var(--ease-out),
    color var(--t-fast) var(--ease-out);
}
.clear:hover {
  background: var(--surface-3);
  color: var(--text);
}
.clear svg {
  width: 14px;
  height: 14px;
}
</style>
