import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import Tooltip from 'primevue/tooltip'
import Aura from '@primeuix/themes/aura'
import { definePreset } from '@primeuix/themes'
import App from './App.vue'
import router from './router'
// Class-based icons (`<i class="pi pi-*">`) and component props both need this stylesheet.
import 'primeicons/primeicons.css'
import './style.css'

const LodestarPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '{indigo.50}',
      100: '{indigo.100}',
      200: '{indigo.200}',
      300: '{indigo.300}',
      400: '{indigo.400}',
      500: '{indigo.500}',
      600: '{indigo.600}',
      700: '{indigo.700}',
      800: '{indigo.800}',
      900: '{indigo.900}',
      950: '{indigo.950}',
    },
  },
})

const savedTheme = localStorage.getItem('lodestar-theme')
const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches
if (savedTheme === 'dark' || (!savedTheme && prefersDark)) document.documentElement.classList.add('dark')

createApp(App)
  .use(router)
  .use(PrimeVue, {
    license: import.meta.env.VITE_PRIMEUI_LICENSE?.trim() || undefined,
    ripple: false,
    theme: {
      preset: LodestarPreset,
      options: {
        darkModeSelector: '.dark',
        cssLayer: {
          name: 'primevue',
          order: 'tailwind-base, primevue, tailwind-utilities',
        },
      },
    },
  })
  .use(ToastService)
  .use(ConfirmationService)
  .directive('tooltip', Tooltip)
  .mount('#app')
