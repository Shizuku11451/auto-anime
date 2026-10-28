<script setup lang="ts">
import { Globe } from '@lucide/vue'
import SettingsSection from './SettingsSection.vue'
import { useSettings } from '@/composables/useSettingsForm'

const { form, info, fieldError } = useSettings()
</script>

<template>
  <SettingsSection id="network" title="网络与同步" :icon="Globe" description="代理只作用于 RSS、番剧搜索、直链下载、封面与 Webhook；下载器 RPC 始终直连">
    <div class="field-row">
      <div class="field">
        <label for="st-proxy">出站代理（留空直连；支持 http、https、socks5）</label>
        <input id="st-proxy" v-model="form.proxy_url" type="text" spellcheck="false" placeholder="http://192.168.1.10:7890" />
        <label v-if="info?.proxy_credentials_set" class="check small">
          <input v-model="form.proxy_clear_credentials" type="checkbox" />
          清除代理账号密码
        </label>
        <p class="field-hint">
          {{
            info?.proxy_credentials_set
              ? '已配置代理认证；地址不变时保存会保留。'
              : '需要认证时可在地址中填写账号密码，保存后不会回显。'
          }}
        </p>
      </div>
      <div class="field">
        <label for="st-poll">RSS 同步周期（分钟）</label>
        <input id="st-poll" v-model.number="form.poll_minutes" type="number" min="5" max="1440" step="1" :aria-invalid="!!fieldError('poll_minutes')" :aria-describedby="fieldError('poll_minutes') ? 'st-poll-error' : undefined" />
        <p v-if="fieldError('poll_minutes')" id="st-poll-error" class="form-msg" role="alert">{{ fieldError('poll_minutes') }}</p>
        <p class="field-hint">修改后从下一轮同步开始生效，最小 5 分钟。</p>
      </div>
    </div>
  </SettingsSection>
</template>

<style scoped>
.check.small { font-size: 13px; margin: 6px 0 4px; }
.field-hint { overflow-wrap: anywhere; }
input[aria-invalid="true"] { border-color: var(--red-border); }
</style>
