import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import test from 'node:test'

const apiRoot = import.meta.dirname

function walk(dir: string): string[] {
  const entries = readdirSync(dir)
  const files: string[] = []
  for (const entry of entries) {
    const fullPath = path.join(dir, entry)
    const stat = statSync(fullPath)
    if (stat.isDirectory()) {
      files.push(...walk(fullPath))
      continue
    }
    if (!fullPath.endsWith('.ts')) continue
    if (fullPath.endsWith('.test.ts')) continue
    files.push(fullPath)
  }
  return files
}

test('frontend api files do not keep literal message fallbacks', () => {
  const offenders: string[] = []
  const patterns = [
    /error\.message\s*\|\|\s*['"]/g,
    /response\.message\s*\|\|\s*response\.error\s*\|\|\s*['"]/g,
  ]

  for (const filePath of walk(apiRoot)) {
    const content = readFileSync(filePath, 'utf8')
    for (const pattern of patterns) {
      if (!pattern.test(content)) continue
      offenders.push(path.relative(apiRoot, filePath))
      break
    }
    patterns.forEach((pattern) => {
      pattern.lastIndex = 0
    })
  }

  assert.deepEqual(offenders, [])
})