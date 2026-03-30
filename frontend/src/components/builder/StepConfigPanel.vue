<script setup lang="ts">
import { computed, ref } from 'vue'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import ExpressionEditor from './ExpressionEditor.vue'
import HumanTaskConfig from './config/HumanTaskConfig.vue'
import AgentConfig from './config/AgentConfig.vue'
import IntegrationConfig from './config/IntegrationConfig.vue'
import RuleConfig from './config/RuleConfig.vue'
import TimerConfig from './config/TimerConfig.vue'
import NotificationConfig from './config/NotificationConfig.vue'
import type { FormSchema } from '../forms/FormRenderer.vue'
import type { WorkflowStep } from './model'

type ConnectorMeta = {
  key: string
  name: string
  actions?: Array<{ key: string; name?: string }>
}

const props = defineProps<{
  step: WorkflowStep | null
  open: boolean
  availableFields: string[]
  connectors: ConnectorMeta[]
  promptTemplates: string[]
}>()

const emit = defineEmits<{
  close: []
  update: [step: WorkflowStep]
  rename: [fromID: string, toID: string]
}>()

const renameTarget = ref<string>('')

const formSchema = computed<FormSchema>(() => {
  const cfg = (props.step?.config ?? {}) as Record<string, unknown>
  return (cfg.form_schema as FormSchema) ?? { title: 'Form', layout: [], actions: [] }
})

const hasMultiDeps = computed(() => (props.step?.depends_on?.length ?? 0) > 1)

function patchStep(patch: Partial<WorkflowStep>) {
  if (!props.step) {
    return
  }
  emit('update', { ...props.step, ...patch })
}

function patchConfig(next: Record<string, unknown>) {
  patchStep({ config: next })
}

function patchForm(next: FormSchema) {
  if (!props.step) {
    return
  }
  patchConfig({ ...(props.step.config ?? {}), form_schema: next })
}

function doRename() {
  if (!props.step || !renameTarget.value) {
    return
  }
  emit('rename', props.step.id, renameTarget.value)
  renameTarget.value = ''
}
</script>

<template>
  <aside class="panel" :class="{ open }">
    <div v-if="step" class="inner">
      <header>
        <h3>Step Config</h3>
        <button type="button" @click="emit('close')">×</button>
      </header>

      <label>Step ID</label>
      <InputText :model-value="step.id" disabled />
      <div class="rename">
        <InputText v-model="renameTarget" placeholder="New step id" />
        <button type="button" @click="doRename">Rename</button>
      </div>

      <label>Display Label</label>
      <InputText
        :model-value="String((step?.config as Record<string, unknown> | undefined)?.label ?? '')"
        @update:model-value="(value) => patchConfig({ ...(step?.config ?? {}), label: value })"
      />

      <label>Guard Condition</label>
      <ExpressionEditor
        :model-value="String(step.condition ?? '')"
        :fields="availableFields"
        @update:model-value="(value) => patchStep({ condition: value })"
      />

      <label v-if="hasMultiDeps">Join Mode</label>
      <select
        v-if="hasMultiDeps"
        :value="String(step.join ?? 'all')"
        @change="(event) => patchStep({ join: (event.target as HTMLSelectElement).value })"
      >
        <option value="all">all</option>
        <option value="any">any</option>
      </select>

      <label>Error Policy Retry Count</label>
      <InputNumber
        :model-value="Number((step?.error_policy as Record<string, unknown> | undefined)?.max_attempts ?? 1)"
        :min="1"
        @update:model-value="(value) => patchStep({ error_policy: { ...(step?.error_policy ?? {}), max_attempts: value ?? 1 } })"
      />

      <label>Error Policy Backoff</label>
      <InputText
        :model-value="String((step?.error_policy as Record<string, unknown> | undefined)?.backoff ?? 'none')"
        @update:model-value="(value) => patchStep({ error_policy: { ...(step?.error_policy ?? {}), backoff: value } })"
      />

      <label>Error Policy On Exhausted</label>
      <InputText
        :model-value="String((step?.error_policy as Record<string, unknown> | undefined)?.on_exhausted ?? 'fail')"
        @update:model-value="(value) => patchStep({ error_policy: { ...(step?.error_policy ?? {}), on_exhausted: value } })"
      />

      <HumanTaskConfig
        v-if="step.type === 'human_task'"
        :config="(step.config ?? {}) as Record<string, unknown>"
        :schema-fields="availableFields"
        :form-schema="formSchema"
        @update="patchConfig"
        @update-form="patchForm"
      />
      <AgentConfig
        v-else-if="step.type === 'agent'"
        :config="(step.config ?? {}) as Record<string, unknown>"
        :templates="promptTemplates"
        @update="patchConfig"
      />
      <IntegrationConfig
        v-else-if="step.type === 'integration'"
        :config="(step.config ?? {}) as Record<string, unknown>"
        :connectors="connectors"
        @update="patchConfig"
      />
      <RuleConfig
        v-else-if="step.type === 'rule'"
        :config="(step.config ?? {}) as Record<string, unknown>"
        @update="patchConfig"
      />
      <TimerConfig
        v-else-if="step.type === 'timer'"
        :config="(step.config ?? {}) as Record<string, unknown>"
        @update="patchConfig"
      />
      <NotificationConfig
        v-else-if="step.type === 'notification'"
        :config="(step.config ?? {}) as Record<string, unknown>"
        @update="patchConfig"
      />
    </div>
  </aside>
</template>

<style scoped>
.panel {
  width: 0;
  overflow: hidden;
  border-left: 1px solid #dbe3ef;
  background: #fff;
  transition: width 0.2s ease;
}

.panel.open {
  width: 360px;
}

.inner {
  display: grid;
  gap: 0.5rem;
  padding: 0.8rem;
}

header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.rename {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 0.35rem;
}
</style>
