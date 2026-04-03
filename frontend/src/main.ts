import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import AceryxPreset from './theme/AceryxPreset'
import { createPinia } from 'pinia'
import router from './router'
import { useAuth } from './composables/useAuth'
import './style.css'
import App from './App.vue'
import 'primeicons/primeicons.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)
app.use(PrimeVue, {
  theme: {
    preset: AceryxPreset,
    options: {
      darkModeSelector: '.p-dark',
      cssLayer: false,
    },
  },
})

void useAuth().initialize()

app.mount('#app')
