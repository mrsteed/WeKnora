export interface LongDocumentArtifactLike extends Record<string, any> {
	id?: string;
	source_message_id?: string;
	content_snapshot?: string;
	revision_no?: number;
	long_document_enabled?: boolean;
}

export interface LongDocumentSessionLike extends Record<string, any> {
	id?: string;
	request_id?: string;
	content?: string;
	final_document_content?: string;
	document_display_mode?: string;
	long_document_enabled?: boolean;
	chat_document_artifact?: LongDocumentArtifactLike | null;
	agentEventStream?: Array<Record<string, any>>;
}

export interface LongDocumentOutlinePreview {
	title: string;
	sections: string[];
	outlineOnly?: boolean;
}

export type LongDocumentDisplayMode = 'full' | 'delta';

export interface LongDocumentArtifactDisplayPayload {
	artifact: LongDocumentArtifactLike | null;
	mode: LongDocumentDisplayMode;
	messageId?: string;
	requestId?: string;
}

export interface LongDocumentPlanningState {
	outline: LongDocumentOutlinePreview | null;
	stage: string;
	progressLabel: string;
	sectionCurrent: number;
	sectionTotal: number;
	sectionTitle: string;
	queryCurrent: number;
	queryTotal: number;
	isPlanningLive: boolean;
	visible: boolean;
}