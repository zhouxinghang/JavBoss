import assert from 'node:assert/strict'
import test from 'node:test'
import { loadModules } from '../../helpers/modules.js'

test('cleanupModel', async (t) => {
  const [model] = await loadModules(t, ['features/directories/cleanupModel.js'])

  await t.test('filters junk items by category and search keyword', () => {
    const items = [
      {
        relative_path: '.DS_Store',
        filename: '.DS_Store',
        category: 'system_junk',
        size: 6148,
        reason_zh: 'macOS 元数据',
        reason_en: 'macOS metadata',
      },
      {
        relative_path: 'sub/扫码进群.png',
        filename: '扫码进群.png',
        category: 'suspected_ad_image',
        size: 40960,
        reason_zh: '文件名含广告/推广关键词',
        reason_en: 'Filename contains ad/promo keywords',
      },
    ]

    assert.equal(model.ALL_JUNK_CATEGORY_IDS.length, 6)
    assert.equal(model.filterJunkItems(items, { categoryFilter: 'system_junk' }).length, 1)
    assert.equal(model.filterJunkItems(items, { search: '扫码' }).length, 1)
    assert.equal(model.filterJunkItems(items, { search: 'keywords' }).length, 1)
    assert.equal(model.filterJunkItems(items, { categoryFilter: 'torrent' }).length, 0)
  })

  await t.test('duplicate quick-select keeps best copy and skips multi-part groups', () => {
    const groups = [
      {
        code: 'IPX-001',
        count: 2,
        total_size: 7000,
        reclaimable_size: 2000,
        has_multi_part: false,
        files: [
          { relative_path: 'a/IPX-001.mp4', size: 5000, modified_at_unix_ms: 1000 },
          { relative_path: 'b/IPX-001-C.mkv', size: 2000, modified_at_unix_ms: 9000 },
        ],
      },
      {
        code: 'SSIS-002',
        count: 2,
        total_size: 6000,
        reclaimable_size: 2500,
        has_multi_part: true,
        files: [
          { relative_path: 'SSIS-002-CD1.mp4', size: 3500, modified_at_unix_ms: 1000 },
          { relative_path: 'SSIS-002-CD2.mp4', size: 2500, modified_at_unix_ms: 2000 },
        ],
      },
    ]

    const keepLargest = model.selectDuplicatePathsKeepingBest(groups, 'largest')
    assert.deepEqual(keepLargest, {
      paths: ['b/IPX-001-C.mkv'],
      skippedMultiPartCount: 1,
    })

    const keepNewest = model.selectDuplicatePathsKeepingBest(groups, 'newest')
    assert.deepEqual(keepNewest, {
      paths: ['a/IPX-001.mp4'],
      skippedMultiPartCount: 1,
    })

    assert.deepEqual(
      model.findFullySelectedDuplicateGroups(groups, new Set(['a/IPX-001.mp4', 'b/IPX-001-C.mkv'])),
      ['IPX-001']
    )
  })

  await t.test(
    'applyDeletedPathsToScanResult updates summaries and drops non-duplicate groups',
    () => {
      const initial = {
        selected_categories: ['system_junk', 'torrent'],
        category_summaries: [
          { category: 'system_junk', count: 1, total_size: 100 },
          { category: 'torrent', count: 1, total_size: 200 },
        ],
        junk_total_count: 2,
        junk_total_size: 300,
        junk_items: [
          { relative_path: '.DS_Store', category: 'system_junk', size: 100 },
          { relative_path: 'a.torrent', category: 'torrent', size: 200 },
        ],
        duplicate_group_count: 2,
        duplicate_file_count: 5,
        duplicate_total_size: 1500,
        duplicate_reclaimable_size: 800,
        duplicate_groups: [
          {
            code: 'IPX-001',
            count: 2,
            total_size: 500,
            reclaimable_size: 200,
            files: [
              { relative_path: 'a/IPX-001.mp4', size: 300 },
              { relative_path: 'b/IPX-001.mp4', size: 200 },
            ],
          },
          {
            code: 'SSIS-002',
            count: 3,
            total_size: 1000,
            reclaimable_size: 600,
            files: [
              { relative_path: '1/SSIS-002.mp4', size: 400 },
              { relative_path: '2/SSIS-002.mp4', size: 350 },
              { relative_path: '3/SSIS-002.mp4', size: 250 },
            ],
          },
        ],
      }

      const updated = model.applyDeletedPathsToScanResult(initial, [
        '.DS_Store',
        'b/IPX-001.mp4',
        '3/SSIS-002.mp4',
      ])

      assert.equal(updated.junk_total_count, 1)
      assert.equal(updated.junk_total_size, 200)
      assert.deepEqual(updated.category_summaries, [
        { category: 'system_junk', count: 0, total_size: 0 },
        { category: 'torrent', count: 1, total_size: 200 },
      ])
      assert.equal(updated.duplicate_group_count, 1)
      assert.equal(updated.duplicate_file_count, 2)
      assert.equal(updated.duplicate_total_size, 750)
      assert.equal(updated.duplicate_reclaimable_size, 350)
      assert.equal(updated.duplicate_groups[0].code, 'SSIS-002')
      assert.equal(updated.duplicate_groups[0].count, 2)
    }
  )
})
