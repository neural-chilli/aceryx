import { definePreset } from '@primevue/themes'
import Aura from '@primevue/themes/aura'

const AceryxPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '{violet.50}',
      100: '{violet.100}',
      200: '{violet.200}',
      300: '{violet.300}',
      400: '{violet.400}',
      500: '#863bff',
      600: '#7429e6',
      700: '#6320cc',
      800: '#5118b3',
      900: '#401099',
      950: '#2e0880',
    },
    colorScheme: {
      light: {
        surface: {
          0: '#ffffff',
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#475569',
          700: '#334155',
          800: '#1e293b',
          900: '#0f172a',
          950: '#020617',
        },
      },
      dark: {
        surface: {
          0: '#f8fafc',
          50: '#e2e8f0',
          100: '#cbd5e1',
          200: '#94a3b8',
          300: '#64748b',
          400: '#475569',
          500: '#334155',
          600: '#283548',
          700: '#1e293b',
          800: '#172033',
          900: '#0f172a',
          950: '#0a1120',
        },
      },
    },
  },
})

export default AceryxPreset
