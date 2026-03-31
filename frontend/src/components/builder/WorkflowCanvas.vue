<script setup lang="ts">
import { computed } from 'vue'
import { VueFlow, useVueFlow, type Connection, type Edge } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import HumanTaskNode from './nodes/HumanTaskNode.vue'
import AgentNode from './nodes/AgentNode.vue'
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

const { screenToFlowCoordinate } = useVueFlow()

const nodes = computed(() => astToNodes(props.ast).map((node) => ({
  ...node,
  class: props.highlightedStepId && node.id === props.highlightedStepId ? 'highlighted' : '',
})))
const edges = computed(() => astToEdges(props.ast))

const nodeTypes: any = {
  human_task: HumanTaskNode,
  agent: AgentNode,
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
  const type = event.dataTransfer?.getData('text/aceryx-step-type')
  if (!type) {
    return
  }
  const position = screenToFlowCoordinate({
    x: event.clientX,
    y: event.clientY,
  })
  addStep(props.ast, type, position)
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
      fit-view-on-init
      @connect="onConnect"
      @nodes-delete="onNodesDelete"
      @edges-delete="onEdgesDelete"
      @node-drag-stop="onNodeDragStop"
      @node-click="onNodeClick"
      @pane-click="onPaneClick"
      @drop="onDrop"
      @dragover="onDragOver"
    />
  </div>
</template>

<style scoped>
.canvas-shell {
  width: 100%;
  height: 100%;
  background:
    radial-gradient(circle at 1px 1px, #cbd5e1 1px, transparent 0);
  background-size: 22px 22px;
}

:deep(.vue-flow__node.highlighted) {
  box-shadow: 0 0 0 3px #f59e0b;
}
</style>
