<template>
  <div v-if="longDocumentEnabled" class="long-document-message-enhancements">
    <template v-if="showPlanning">
      <LongDocumentPlanningTimeline
        :visible="timelineVisible"
        :items="timelineItems"
      />
      <LongDocumentOutlinePanel
        :visible="planningVisible"
        :outline="planningOutline"
        :stage="planningStage"
        :progress-label="planningProgressLabel"
        :section-title="planningSectionTitle"
      />
    </template>
    <ChatDocumentArtifactCard
      v-if="showArtifact && artifact"
      :artifact="artifact"
      :selected-artifact-id="selectedArtifactId"
      :preview-content="previewContent"
      :export-content="exportContent"
      :can-toggle-document-display="canToggleDocumentDisplay"
      :document-display-mode="documentDisplayMode"
      :export-api-base="exportApiBase"
      @view-revisions="$emit('view-artifact-revisions', $event)"
      @use-as-base="$emit('use-artifact-as-base', $event)"
      @clear-base="$emit('clear-artifact-base', $event)"
      @toggle-document-display="handleToggleDocumentDisplay"
    />
  </div>
</template>

<script setup>
import { computed } from 'vue';

import ChatDocumentArtifactCard from '../../components/ChatDocumentArtifactCard.vue';

import { useLongDocumentArtifacts } from '../useLongDocumentArtifacts';
import { useLongDocumentPlanning } from '../useLongDocumentPlanning';
import { useLongDocumentPlanningTimeline } from '../useLongDocumentPlanningTimeline';

import LongDocumentOutlinePanel from './LongDocumentOutlinePanel.vue';
import LongDocumentPlanningTimeline from './LongDocumentPlanningTimeline.vue';

const props = defineProps({
  session: {
    type: Object,
    default: null,
  },
  content: {
    type: String,
    default: '',
  },
  selectedArtifactId: {
    type: String,
    default: '',
  },
  exportApiBase: {
    type: String,
    default: '',
  },
  showPlanning: {
    type: Boolean,
    default: true,
  },
  showArtifact: {
    type: Boolean,
    default: true,
  },
});

const emit = defineEmits([
  'view-artifact-revisions',
  'use-artifact-as-base',
  'clear-artifact-base',
  'artifact-display-update',
]);

const sessionRef = computed(() => props.session || null);
const contentRef = computed(() => props.content || props.session?.content || '');

const {
  artifact,
  longDocumentEnabled,
  documentDisplayMode,
  previewContent,
  exportContent,
  canToggleDocumentDisplay,
  buildDisplayPayload,
} = useLongDocumentArtifacts(sessionRef, contentRef);

const {
  outline: planningOutline,
  stage: planningStage,
  progressLabel: planningProgressLabel,
  sectionTitle: planningSectionTitle,
  visible: planningVisible,
} = useLongDocumentPlanning(sessionRef);

const {
	visible: timelineVisible,
	timelineItems,
} = useLongDocumentPlanningTimeline(sessionRef);

const handleToggleDocumentDisplay = () => {
  const nextMode = documentDisplayMode.value === 'full' ? 'delta' : 'full';
  emit('artifact-display-update', buildDisplayPayload(nextMode));
};
</script>

<style lang="less" scoped>
.long-document-message-enhancements {
  display: flex;
  flex-direction: column;
}
</style>