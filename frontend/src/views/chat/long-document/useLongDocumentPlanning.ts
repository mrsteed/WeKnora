import { computed, type ComputedRef } from 'vue';

import {
	extractPlanningOutlineFromCompleteEvent,
	getPlanningOutlineFromEvent,
	getPlanningOutlineFromThinkingEvent,
} from '../utils/planningOutline';

import type { LongDocumentSessionLike } from './longDocumentTypes';

const trimText = (value: unknown): string => typeof value === 'string' ? value.trim() : '';

const normalizeCount = (value: unknown): number => {
	const numeric = Number(value);
	return Number.isFinite(numeric) ? numeric : 0;
};

export function useLongDocumentPlanning(session: ComputedRef<LongDocumentSessionLike | null | undefined>) {
	const eventStream = computed<Array<Record<string, any>>>(() => (
		Array.isArray(session.value?.agentEventStream) ? session.value?.agentEventStream || [] : []
	));

	const latestProgressEvent = computed<Record<string, any> | null>(() => {
		for (let index = eventStream.value.length - 1; index >= 0; index -= 1) {
			const event = eventStream.value[index];
			if (!event || event.type !== 'thinking') {
				continue;
			}
			if (
				trimText(event.stage) ||
				trimText(event.progress_label) ||
				normalizeCount(event.section_total) > 0 ||
				normalizeCount(event.query_total) > 0
			) {
				return event;
			}
		}
		return null;
	});

	const outline = computed(() => {
		for (let index = eventStream.value.length - 1; index >= 0; index -= 1) {
			const event = eventStream.value[index];
			if (!event) {
				continue;
			}
			const content = trimText(event.content);
			if (event.type === 'thinking') {
				const fromThinking = getPlanningOutlineFromThinkingEvent(event, content);
				if (fromThinking) {
					return fromThinking;
				}
				const fromEvent = getPlanningOutlineFromEvent(event, content);
				if (fromEvent) {
					return fromEvent;
				}
				continue;
			}
			if (event.type === 'agent_complete') {
				const fromComplete = extractPlanningOutlineFromCompleteEvent(event);
				if (fromComplete) {
					return fromComplete;
				}
				const fromEvent = getPlanningOutlineFromEvent(event, content);
				if (fromEvent) {
					return fromEvent;
				}
			}
		}
		return null;
	});

	const latestCompleteEvent = computed<Record<string, any> | null>(() => {
		for (let index = eventStream.value.length - 1; index >= 0; index -= 1) {
			const event = eventStream.value[index];
			if (event?.type === 'agent_complete') {
				return event;
			}
		}
		return null;
	});

	const stage = computed(() => {
		const progressStage = trimText(latestProgressEvent.value?.stage);
		const completionStatus = trimText(latestCompleteEvent.value?.document_generation_status);
		if (completionStatus === 'completed') {
			return 'completed';
		}
		if (completionStatus === 'needs_review') {
			return 'needs_review';
		}
		if (completionStatus === 'blocked') {
			return 'blocked';
		}
		if (outline.value && progressStage !== '' && progressStage !== 'planning') {
			return 'planned';
		}
		return progressStage;
	});
	const progressLabel = computed(() => trimText(latestProgressEvent.value?.progress_label));
	const sectionCurrent = computed(() => normalizeCount(latestProgressEvent.value?.section_current));
	const sectionTotal = computed(() => normalizeCount(latestProgressEvent.value?.section_total));
	const sectionTitle = computed(() => trimText(latestProgressEvent.value?.section_title));
	const queryCurrent = computed(() => normalizeCount(latestProgressEvent.value?.query_current));
	const queryTotal = computed(() => normalizeCount(latestProgressEvent.value?.query_total));
	const isPlanningLive = computed(() => ['planning', 'retrieving', 'generating', 'finalizing', 'document_edit', 'queued', 'planned'].includes(stage.value));
	const visible = computed(() => (
		session.value?.long_document_enabled === true && (Boolean(outline.value) || isPlanningLive.value || Boolean(latestProgressEvent.value) || Boolean(latestCompleteEvent.value))
	));

	return {
		outline,
		stage,
		progressLabel,
		sectionCurrent,
		sectionTotal,
		sectionTitle,
		queryCurrent,
		queryTotal,
		isPlanningLive,
		visible,
	};
}