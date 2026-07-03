import { computed, type ComputedRef } from 'vue';

import { resolveChatExportContent } from '@/utils/exportUtils';

import type {
	LongDocumentArtifactDisplayPayload,
	LongDocumentDisplayMode,
	LongDocumentSessionLike,
} from './longDocumentTypes';

const trimText = (value: unknown): string => typeof value === 'string' ? value.trim() : '';

export function useLongDocumentArtifacts(
	session: ComputedRef<LongDocumentSessionLike | null | undefined>,
	content: ComputedRef<string | undefined>,
) {
	const artifact = computed(() => session.value?.chat_document_artifact || null);
	const longDocumentEnabled = computed(() => session.value?.long_document_enabled === true);
	const documentDisplayMode = computed<LongDocumentDisplayMode>(() => (
		session.value?.document_display_mode === 'full' ? 'full' : 'delta'
	));
	const deltaContent = computed(() => trimText(content.value) || trimText(session.value?.content));
	const fullContent = computed(() => trimText(session.value?.final_document_content) || trimText(artifact.value?.content_snapshot));
	const previewContent = computed(() => (
		documentDisplayMode.value === 'full'
			? (fullContent.value || deltaContent.value)
			: deltaContent.value
	));
	const exportContent = computed(() => resolveChatExportContent(
		session.value || null,
		fullContent.value || deltaContent.value,
	));
	const hasArtifact = computed(() => Boolean(artifact.value?.id));
	const canToggleDocumentDisplay = computed(() => hasArtifact.value);

	const buildDisplayPayload = (mode: LongDocumentDisplayMode): LongDocumentArtifactDisplayPayload => ({
		artifact: artifact.value,
		mode,
		messageId: session.value?.id ? String(session.value.id) : '',
		requestId: session.value?.request_id ? String(session.value.request_id) : '',
	});

	return {
		artifact,
		longDocumentEnabled,
		documentDisplayMode,
		deltaContent,
		fullContent,
		previewContent,
		exportContent,
		hasArtifact,
		canToggleDocumentDisplay,
		buildDisplayPayload,
	};
}