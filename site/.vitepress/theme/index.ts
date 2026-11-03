import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import LandingPage from './LandingPage.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('LandingPage', LandingPage)
  },
} satisfies Theme
