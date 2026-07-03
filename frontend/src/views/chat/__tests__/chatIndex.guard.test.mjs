import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const here = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(join(here, '..', 'index.vue'), 'utf8')

test('chat routing uses the anti-drift agent stream getter', () => {
  assert.match(source, /return useSettingsStoreInstance\.isAgentStreamMode;/)
  assert.doesNotMatch(source, /return useSettingsStoreInstance\.isAgentEnabled;/)
})

test('session title updates are broadcast to sidebar buckets immediately', () => {
  assert.match(
    source,
    /window\.dispatchEvent\(new CustomEvent\('session-title-updated', \{\s*detail: \{ sessionId, title \}/,
  )
})