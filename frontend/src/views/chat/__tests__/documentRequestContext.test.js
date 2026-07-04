import test from 'node:test';
import assert from 'node:assert/strict';

import {
  buildUserDocumentRequestContext,
  resolveExplicitBaseArtifact,
} from '../utils/documentRequestContext.js';

test('auto-selected baseline does not become an explicit base artifact', () => {
  const artifact = resolveExplicitBaseArtifact({
    id: 'artifact-1',
    session_id: 'session-1',
  }, false, 'session-1');

  assert.equal(artifact, null);
});

test('locked baseline is preserved as routing context without forcing document intent', () => {
  const explicitBaseArtifact = resolveExplicitBaseArtifact({
    id: 'artifact-1',
    session_id: 'session-1',
  }, true, 'session-1');

  const request = buildUserDocumentRequestContext({
    inferredIntentHint: 'normal',
    explicitBaseArtifact,
    latestArtifact: null,
  });

  assert.deepEqual(request, {
    intentHint: 'normal',
    isDocumentEditIntent: false,
    requestIntentHint: undefined,
    baseArtifactId: 'artifact-1',
    documentOutputMode: '',
  });
});

test('explicit revise intent keeps delta mode and uses the latest artifact when no locked base exists', () => {
  const request = buildUserDocumentRequestContext({
    inferredIntentHint: 'revise_document',
    explicitBaseArtifact: null,
    latestArtifact: { id: 'artifact-2' },
  });

  assert.deepEqual(request, {
    intentHint: 'revise_document',
    isDocumentEditIntent: true,
    requestIntentHint: 'revise_document',
    baseArtifactId: 'artifact-2',
    documentOutputMode: 'delta_only',
  });
});

test('locked baseline takes precedence over an automatically discovered artifact', () => {
  const request = buildUserDocumentRequestContext({
    inferredIntentHint: 'continue_document',
    explicitBaseArtifact: { id: 'artifact-locked' },
    latestArtifact: { id: 'artifact-auto' },
  });

  assert.equal(request.baseArtifactId, 'artifact-locked');
  assert.equal(request.documentOutputMode, 'delta_only');
  assert.equal(request.requestIntentHint, 'continue_document');
});
