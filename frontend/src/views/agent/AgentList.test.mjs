import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(join(__dirname, 'AgentList.vue'), 'utf8')

test('openCreateModal goes through ensureAgentCreateReady first', () => {
  assert.match(source, /const ensureAgentCreateReady = async \(\) => \{[\s\S]*?return false[\s\S]*?return true[\s\S]*?\}/)
  assert.match(source, /const openCreateModal = async \(\) => \{[\s\S]*?const ready = await ensureAgentCreateReady\(\)[\s\S]*?if \(!ready\) return false[\s\S]*?editorVisible\.value = true[\s\S]*?return true[\s\S]*?\}/)
})

test('handleCreateAgent only marks the guide after the guarded opener succeeds', () => {
  assert.match(source, /const handleCreateAgent = async \(\) => \{[\s\S]*?const opened = await openCreateModal\(\)[\s\S]*?if \(!opened\) return[\s\S]*?markContextualGuideDone\('agentList'\)[\s\S]*?\}/)
})