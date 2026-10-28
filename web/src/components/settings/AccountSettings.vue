<script setup lang="ts">
import { UserRound } from '@lucide/vue'
import SettingsSection from './SettingsSection.vue'
import { useSettings } from '@/composables/useSettingsForm'

const { form, info, fieldError } = useSettings()
</script>

<template>
  <SettingsSection id="account" title="管理员账户" :icon="UserRound" description="启用后访问面板需要登录；修改账户或密码后所有会话退出">
    <div class="field-row">
      <div class="field">
        <label for="st-user">用户名</label>
        <input id="st-user" v-model="form.web_user" type="text" autocomplete="username" :aria-invalid="!!fieldError('web_user')" :aria-describedby="fieldError('web_user') ? 'st-user-error' : undefined" />
        <p v-if="fieldError('web_user')" id="st-user-error" class="form-msg" role="alert">{{ fieldError('web_user') }}</p>
      </div>
      <div class="field">
        <label for="st-pass">新密码{{ info?.web_password_set ? '（留空保持不变）' : '' }}</label>
        <input id="st-pass" v-model="form.web_password" type="password" autocomplete="new-password" placeholder="至少 6 位" :aria-invalid="!!fieldError('web_password')" :aria-describedby="fieldError('web_password') ? 'st-pass-error' : undefined" />
        <p v-if="fieldError('web_password')" id="st-pass-error" class="form-msg" role="alert">{{ fieldError('web_password') }}</p>
      </div>
    </div>
    <p class="field-hint">设置过账户后无法在页面撤销登录；修改用户名时必须同时填写新密码。</p>
  </SettingsSection>
</template>

<style scoped>
input[aria-invalid="true"] { border-color: var(--red-border); }
</style>
