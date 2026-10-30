import assert from 'node:assert/strict'
import test from 'node:test'
import { loadModules } from '../helpers/modules.js'

test('Chinese work titles take priority across display and page titles', async (t) => {
  const [{ getJavDisplayTitle }, { buildJavDetailPageTitle }] = await loadModules(t, [
    'utils/jav.js',
    'navigation/pageTitle.js',
  ])
  const item = { code: 'ABC-001', title: 'Original title', zh_title: ' 中文标题 ' }
  assert.equal(getJavDisplayTitle(item), '中文标题')
  assert.equal(buildJavDetailPageTitle(item), 'ABC-001 中文标题')
  assert.equal(item.title, 'Original title')
  assert.equal(getJavDisplayTitle({ ...item, zh_title: ' ' }), 'Original title')
  assert.equal(buildJavDetailPageTitle({ ...item, zh_title: ' ' }), 'ABC-001 Original title')
  assert.equal(getJavDisplayTitle({ code: 'ABC-001', title: ' ' }), 'ABC-001')
  assert.ok(getJavDisplayTitle(null))
})
