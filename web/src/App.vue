<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterView } from 'vue-router'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import MobileHeader from '@/components/layout/MobileHeader.vue'
import MobileTabBar from '@/components/layout/MobileTabBar.vue'
import ToastHost from '@/components/ui/ToastHost.vue'
import ConfirmHost from '@/components/ui/ConfirmHost.vue'
import DialogHost from '@/dialogs/DialogHost.vue'
import { useAppStore } from '@/stores/app'
import { useUpdateStore } from '@/stores/update'

const store = useAppStore()
const updateStore = useUpdateStore()
onMounted(() => {
  store.startPolling()
  updateStore.startUpdateWatch()
})
</script>

<template>
  <div class="shell">
    <a class="skip-link" href="#main-content">跳到主要内容</a>
    <AppSidebar class="desktop-only" />
    <MobileHeader class="mobile-only" />
    <main id="main-content" class="main" tabindex="-1">
      <div v-if="store.loadError.value" class="connection-error" role="alert">
        <span>数据更新失败，当前内容可能不是最新的。{{ store.loadError.value }}</span>
        <button type="button" @click="store.refresh()">重新加载</button>
      </div>
      <RouterView v-slot="{ Component, route }">
        <Transition name="page" mode="out-in">
          <component :is="Component" :key="route.name" />
        </Transition>
      </RouterView>
    </main>
    <MobileTabBar class="mobile-only" />
  </div>
  <DialogHost />
  <ConfirmHost />
  <ToastHost />
</template>

<style scoped>
.shell {
  display: flex;
  min-height: 100dvh;
}
.main {
  flex: 1;
  min-width: 0;
  padding: 42px clamp(24px, 3.2vw, 56px) 56px;
}
.connection-error { max-width: var(--page-max); margin: 0 auto 20px; padding: 12px 16px; background: var(--red-soft); border: 1px solid var(--red-border); border-radius: var(--r-sm); color: var(--red); display: flex; flex-wrap: wrap; gap: 12px; justify-content: space-between; overflow-wrap: anywhere; }
.connection-error button { border: 0; background: transparent; color: inherit; text-decoration: underline; cursor: pointer; }
.skip-link { position: fixed; top: 8px; left: 8px; z-index: 200; padding: 10px 16px; background: var(--surface); border: 1px solid var(--accent); border-radius: var(--r-sm); transform: translateY(-160%); }
.skip-link:focus { transform: none; }
/* 只负责显隐，不改写组件自己的 display（底部标签栏是 grid，之前被 flex !important 覆盖） */
@media (min-width: 901px) {
  .mobile-only {
    display: none !important;
  }
}
@media (max-width: 900px) {
  .shell {
    flex-direction: column;
  }
  .desktop-only {
    display: none !important;
  }
  .main {
    padding: 20px 16px calc(92px + env(safe-area-inset-bottom));
  }
}
</style>
