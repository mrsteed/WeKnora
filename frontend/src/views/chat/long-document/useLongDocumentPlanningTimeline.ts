import { computed, type ComputedRef } from 'vue';

import type { LongDocumentSessionLike } from './longDocumentTypes';

const trimText = (value: unknown): string => typeof value === 'string' ? value.trim() : '';

const stageLabelMap: Record<string, string> = {
	queued: '任务入队',
	planning: '文档规划',
	retrieving: '检索知识库',
	generating: '生成正文',
	finalizing: '收尾整理',
	document_edit: '文档修订',
};

export function useLongDocumentPlanningTimeline(session: ComputedRef<LongDocumentSessionLike | null | undefined>) {
	const eventStream = computed<Array<Record<string, any>>>(() => (
		Array.isArray(session.value?.agentEventStream) ? session.value?.agentEventStream || [] : []
	));

	const timelineItems = computed(() => {
		const result: Array<Record<string, any>> = [];
		for (const event of eventStream.value) {
			if (!event || event.type !== 'thinking') {
				continue;
			}
			if (event.long_document_enabled !== true) {
				continue;
			}
			const stage = trimText(event.stage) || 'planning';
			const content = trimText(event.content);
			const progressLabel = trimText(event.progress_label);
			const outline = event.planning_outline && typeof event.planning_outline === 'object'
				? event.planning_outline
				: event.outline && typeof event.outline === 'object'
					? event.outline
					: null;
			if (!content && !progressLabel && !outline) {
				continue;
			}
			result.push({
				id: String(event.event_id || `${stage}-${result.length}`),
				stage,
				label: stageLabelMap[stage] || '处理中',
				content,
				progressLabel,
				outline,
				sectionTitle: trimText(event.section_title),
				sectionCurrent: Number.isFinite(Number(event.section_current)) ? Number(event.section_current) : 0,
				sectionTotal: Number.isFinite(Number(event.section_total)) ? Number(event.section_total) : 0,
				done: event.done === true,
				synthetic: event.synthetic === true,
			});
		}
		return result;
	});

	const visible = computed(() => (
		session.value?.long_document_enabled === true && timelineItems.value.length > 0
	));

	return {
		timelineItems,
		visible,
	};
}