import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./editorResources.ts', import.meta.url), 'utf8')

test('agent editor prefetch uses viewer-safe storage status loading', () => {
  assert.match(source, /async function ensureStorageEngineStatus\(force = false\): Promise<void> \{/)

  const prefetchBody = source.match(
    /async function prefetchAgentEditorDeps\(force = false\): Promise<void> \{([\s\S]*?)^  \}/m,
  )?.[1]

  assert.ok(prefetchBody, 'expected to find prefetchAgentEditorDeps')
  assert.match(prefetchBody, /ensureStorageEngineStatus\(force\)/)
  assert.doesNotMatch(prefetchBody, /ensureStorageEngine\(force\)/)
})