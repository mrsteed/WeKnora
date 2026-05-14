export interface PlanningOutlinePreview {
  title: string;
  sections: string[];
  outlineOnly: boolean;
}

export function normalizePlanningOutlineMarkdown(content: string): string;
export function normalizePlanningOutlineSections(sections: unknown): string[];
export function normalizePlanningOutlinePreview(outline: Partial<PlanningOutlinePreview> | null | undefined): PlanningOutlinePreview | null;
export function normalizeStructuredPlanningOutline(rawOutline: unknown): { title: string; sections: string[] } | null;
export function extractPlanningOutlineFromText(content: string): PlanningOutlinePreview | null;
export function extractStructuredPlanningOutlineFromText(content: string): { title: string; sections: string[] } | null;
export function extractPlanningOutlineFromStructured(rawOutline: unknown): PlanningOutlinePreview | null;
export function pickStructuredPlanningOutline(...candidates: unknown[]): { title: string; sections: string[] } | null;
export function extractPlanningOutlineFromCompleteEvent(event: unknown): PlanningOutlinePreview | null;
export function getPlanningOutlineFromEvent(event: unknown, fallbackContent?: string): PlanningOutlinePreview | null;
export function getPlanningOutlineFromThinkingEvent(event: unknown, fallbackContent?: string): PlanningOutlinePreview | null;
export function shouldAllowPlanningOutlineArtifactFallback(input?: {completeEvent?: any;eventStream?: any[] | null;}): boolean;
