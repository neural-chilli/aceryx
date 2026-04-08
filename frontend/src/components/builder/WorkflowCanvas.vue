<script setup lang="ts">
import { computed, onMounted, type Component } from 'vue'
import { VueFlow, useVueFlow, type Connection, type Edge } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import HumanTaskNode from './nodes/HumanTaskNode.vue'
import AgentNode from './nodes/AgentNode.vue'
import AIComponentNode from './nodes/AIComponentNode.vue'
import ExtractionNode from './nodes/ExtractionNode.vue'
import IntegrationNode from './nodes/IntegrationNode.vue'
import RuleNode from './nodes/RuleNode.vue'
import TimerNode from './nodes/TimerNode.vue'
import NotificationNode from './nodes/NotificationNode.vue'
import UnknownNode from './nodes/UnknownNode.vue'
import { addDependencyEdge, addStep, astToEdges, astToNodes, deleteStep, removeDependencyEdge, type WorkflowAST } from './model'

const props = defineProps<{
  ast: WorkflowAST
  highlightedStepId?: string
}>()

const emit = defineEmits<{
  'update:ast': [ast: WorkflowAST]
  selectStep: [stepID: string | null]
}>()

const VIEWPORT_KEY = 'acx:canvas-viewport'
const { screenToFlowCoordinate, setViewport, getViewport, fitView } = useVueFlow()

function saveViewport() {
  try {
    const vp = getViewport()
    localStorage.setItem(VIEWPORT_KEY, JSON.stringify(vp))
  } catch {
    /* storage unavailable */
  }
}

onMounted(() => {
  try {
    const raw = localStorage.getItem(VIEWPORT_KEY)
    if (!raw) {
      fitView()
      return
    }
    const vp = JSON.parse(raw) as { x: number; y: number; zoom: number }
    if (typeof vp.x === 'number' && typeof vp.y === 'number' && typeof vp.zoom === 'number') {
      setViewport(vp)
    } else {
      fitView()
    }
  } catch {
    fitView()
  }
})

const nodes = computed(() => astToNodes(props.ast).map((node) => ({
  ...node,
  class: props.highlightedStepId && node.id === props.highlightedStepId ? 'highlighted' : '',
})))
const edges = computed(() => astToEdges(props.ast))

const nodeTypes: Record<string, Component> = {
  human_task: HumanTaskNode,
  agent: AgentNode,
  ai_component: AIComponentNode,
  extraction: ExtractionNode,
  integration: IntegrationNode,
  rule: RuleNode,
  timer: TimerNode,
  notification: NotificationNode,
  unknown: UnknownNode,
}

function updateAst() {
  emit('update:ast', { ...props.ast, steps: [...props.ast.steps] })
}

function onConnect(connection: Connection) {
  if (!connection.source || !connection.target) {
    return
  }
  addDependencyEdge(props.ast, connection.source, connection.target)
  updateAst()
}

function onEdgesDelete(removed: Edge[]) {
  for (const edge of removed) {
    removeDependencyEdge(props.ast, edge.source, edge.target)
  }
  updateAst()
}

function onNodesDelete(removed: Array<{ id: string }>) {
  for (const node of removed) {
    deleteStep(props.ast, node.id)
  }
  updateAst()
}

function onNodeDragStop(event: { node?: { id: string; position: { x: number; y: number } } }) {
  if (!event.node) {
    return
  }
  const step = props.ast.steps.find((candidate) => candidate.id === event.node?.id)
  if (!step) {
    return
  }
  step.position = { x: event.node.position.x, y: event.node.position.y }
  updateAst()
}

function onNodeClick(event: { node: { id: string } }) {
  emit('selectStep', event.node.id)
}

function onPaneClick() {
  emit('selectStep', null)
}

function onDrop(event: DragEvent) {
  event.preventDefault()
  const rawPayload = event.dataTransfer?.getData('text/aceryx-step-payload')?.trim()
  const fallbackType = event.dataTransfer?.getData('text/aceryx-step-type')?.trim()
  let payload: { type: string; config?: Record<string, unknown> } | null = null
  if (rawPayload) {
    try {
      payload = JSON.parse(rawPayload) as { type: string; config?: Record<string, unknown> }
    } catch {
      payload = null
    }
  }
  const type = payload?.type || fallbackType
  if (!type) {
    return
  }
  const position = screenToFlowCoordinate({
    x: event.clientX,
    y: event.clientY,
  })
  const id = addStep(props.ast, type, position)
  if (payload?.config) {
    const step = props.ast.steps.find((candidate) => candidate.id === id)
    if (step) {
      step.config = { ...(step.config ?? {}), ...payload.config }
    }
  }
  updateAst()
}

function onDragOver(event: DragEvent) {
  event.preventDefault()
}
</script>

<template>
  <div class="canvas-shell">
    <VueFlow
      :nodes="nodes"
      :edges="edges"
      :node-types="nodeTypes"
      :snap-to-grid="true"
      :snap-grid="[20, 20]"
      :default-edge-options="{ type: 'smoothstep' }"
      @connect="onConnect"
      @nodes-delete="onNodesDelete"
      @edges-delete="onEdgesDelete"
      @node-drag-stop="onNodeDragStop"
      @node-click="onNodeClick"
      @pane-click="onPaneClick"
      @drop="onDrop"
      @dragover="onDragOver"
      @viewport-change-end="saveViewport"
    />
  </div>
</template>

<style scoped>
.canvas-shell {
  width: 100%;
  height: 100%;
  background-color: var(--acx-surface-50);
  background-image:
    radial-gradient(circle at 1px 1px, var(--acx-surface-300) 1px, transparent 0);
  background-size: 22px 22px;
}

:deep(.vue-flow__node.highlighted) {
  box-shadow: 0 0 0 3px #f59e0b;
}
</style>
