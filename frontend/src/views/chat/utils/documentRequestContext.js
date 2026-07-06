export function resolveExplicitBaseArtifact(selectedBaseArtifact, selectedBaseArtifactDisplayLocked, sessionId) {
  void selectedBaseArtifactDisplayLocked;
  if (!selectedBaseArtifact?.id) {
    return null;
  }
  if (selectedBaseArtifact?.session_id !== sessionId) {
    return null;
  }
  return selectedBaseArtifact;
}

export function buildUserDocumentRequestContext({
  inferredIntentHint = 'normal',
  explicitBaseArtifact = null,
  latestArtifact = null,
} = {}) {
  const intentHint = inferredIntentHint === 'continue_document' || inferredIntentHint === 'revise_document'
    ? inferredIntentHint
    : 'normal';
  const isDocumentEditIntent = intentHint === 'continue_document' || intentHint === 'revise_document';

  return {
    intentHint,
    isDocumentEditIntent,
    requestIntentHint: intentHint === 'normal' ? undefined : intentHint,
    baseArtifactId: explicitBaseArtifact?.id || (isDocumentEditIntent ? latestArtifact?.id : undefined),
    documentOutputMode: isDocumentEditIntent ? 'delta_only' : '',
  };
}
