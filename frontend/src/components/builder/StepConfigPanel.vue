<script setup lang="ts">
import { computed, ref } from 'vue'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Drawer from 'primevue/drawer'
import ExpressionEditor from './ExpressionEditor.vue'
import HumanTaskConfig from './config/HumanTaskConfig.vue'
import AgentConfig from './config/AgentConfig.vue'
import AIComponentConfig from './config/AIComponentConfig.vue'
import ExtractionConfig from './config/ExtractionConfig.vue'
import IntegrationConfig from './config/IntegrationConfig.vue'
import RuleConfig from './config/RuleConfig.vue'
import TimerConfig from './config/TimerConfig.vue'
import NotificationConfig from './config/NotificationConfig.vue'
import type { WorkflowStep } from './model'

type ConnectorMeta = {
  key: string
  name: string
  actions?: Array<{
    key: string
    name?: string
    input_schema?: Record<string, unknown>
    output_schema?: Record<string, unknown>
  }>
}

type AIComponentMeta = {
  id: string
  display_label: string
  category?: string
  icon?: string
  config_fields?: Array<{
    name: string
    type: string
    label?: string
    required?: boolean
    default?: unknown
    options?: string[]
  }>
}

type ExtractionSchemaMeta = {
  id: string
  name: string
  status?: string
  version?: number
}

const props = defineProps<{
  step: WorkflowStep | null
  open: boolean
  availableFields: string[]
  connectors: ConnectorMeta[]
  promptTemplates: string[]
  aiComponents: AIComponentMeta[]
  extractionSchemas: ExtractionSchemaMeta[]
}>()

const emit = defineEmits<{
  close: []
  update: [step: WorkflowStep]
  rename: [fromID: string, toID: string]
}>()

const renameTarget = ref<string>('')

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
function patchRuleConfig(next: Record<string, unknown>) {
  patchStep({ config: next })
}

function patchRuleOutcomes(next: Record<string, string | string[]>) {
  patchStep({ outcomes: next })
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
  <Drawer
    :visible="open"
    position="right"
    :modal="false"
    :dismissable="false"
    :append-to="'self'"
    :pt="{ root: { style: { width: '480px' } } }"
    @update:visible="(v: boolean) => { if (!v) emit('close') }"
  >
    <template #header>
      <h3 style="margin: 0">Step Config</h3>
    </template>
    <div v-if="step" class="inner">
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
        @update="patchConfig"
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
        :outcomes="step.outcomes ?? {}"
        @update-config="patchRuleConfig"
        @update-outcomes="patchRuleOutcomes"
      />
      <AIComponentConfig
        v-else-if="step.type === 'ai_component'"
        :config="(step.config ?? {}) as Record<string, unknown>"
        :ai-components="aiComponents"
        @update="patchConfig"
      />
      <ExtractionConfig
        v-else-if="step.type === 'extraction' || step.type === 'document_extraction' || step.type === 'doc_extraction'"
        :config="(step.config ?? {}) as Record<string, unknown>"
        :schemas="extractionSchemas"
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
  </Drawer>
</template>

<style scoped>
.inner {
  display: grid;
  gap: 0.5rem;
}

.rename {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 0.35rem;
}
</style>
