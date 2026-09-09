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

const store = useAppStore()
onMounted(() => store.startPolling())
</script>

<template>
  <div class="shell">
    <AppSidebar class="desktop-only" />
    <MobileHeader class="mobile-only" />
    <main class="main">
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
  padding: 34px 40px 56px;
}
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
