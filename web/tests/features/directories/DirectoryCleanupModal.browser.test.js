import assert from 'node:assert/strict'
import test from 'node:test'
import { browserUnavailable, openBrowser } from '../../helpers/browser.js'

test(
  'directory tools opens junk file cleanup and supports scanning and deleting junk and duplicates',
  { skip: browserUnavailable, timeout: 60000 },
  async (t) => {
    const { origin, command, evaluate, waitFor } = await openBrowser(t)
    await command('Page.navigate', { url: `${origin}/tests/fixtures/app.html?view=video` })
    await waitFor(`document.querySelector('aside button[aria-label="Settings"]')`)

    await evaluate(`{
      window.cleanupRequests = [];
      const originalFetch = window.fetch;
      window.fetch = async (input, init = {}) => {
        const url = new URL(input, location.origin);
        if (url.pathname === '/directories/1/cleanup/scan') {
          const body = JSON.parse(init.body || '{}');
          window.cleanupRequests.push({ endpoint: 'scan', body });
          return Response.json({
            directory_id: 1,
            scanned_files: 12,
            duration_ms: 18,
            selected_categories: body.categories || [
              'system_junk',
              'text_web',
              'torrent',
              'windows_shortcut',
              'suspected_ad_image',
              'download_leftover',
            ],
            include_duplicates: body.include_duplicates !== false,
            junk_total_count: 2,
            junk_total_size: 4608,
            category_summaries: [
              { category: 'system_junk', count: 1, total_size: 512 },
              { category: 'text_web', count: 0, total_size: 0 },
              { category: 'torrent', count: 1, total_size: 4096 },
              { category: 'windows_shortcut', count: 0, total_size: 0 },
              { category: 'suspected_ad_image', count: 0, total_size: 0 },
              { category: 'download_leftover', count: 0, total_size: 0 },
            ],
            junk_items: [
              {
                relative_path: '.DS_Store',
                filename: '.DS_Store',
                category: 'system_junk',
                size: 512,
                modified_at_unix_ms: 1710000000000,
                reason_zh: 'macOS 目录元数据文件 (.DS_Store)',
                reason_en: 'macOS folder metadata (.DS_Store)',
              },
              {
                relative_path: 'pack/IPX-001.torrent',
                filename: 'IPX-001.torrent',
                category: 'torrent',
                size: 4096,
                modified_at_unix_ms: 1710000000000,
                reason_zh: '种子/下载描述文件 (.torrent)',
                reason_en: 'Torrent/download descriptor (.torrent)',
              },
            ],
            duplicate_group_count: 2,
            duplicate_file_count: 4,
            duplicate_total_size: 16000000,
            duplicate_reclaimable_size: 6000000,
            duplicate_groups: [
              {
                code: 'IPX-001',
                title: 'Test Title',
                count: 2,
                total_size: 9000000,
                reclaimable_size: 3000000,
                has_multi_part: false,
                files: [
                  {
                    relative_path: 'hd/IPX-001.mp4',
                    filename: 'IPX-001.mp4',
                    size: 6000000,
                    modified_at_unix_ms: 1710000000000,
                    code: 'IPX-001',
                    code_source: 'jav',
                    resolution: '1920x1080',
                    duration_sec: 7200,
                  },
                  {
                    relative_path: 'sd/IPX-001-low.mp4',
                    filename: 'IPX-001-low.mp4',
                    size: 3000000,
                    modified_at_unix_ms: 1710000000000,
                    code: 'IPX-001',
                    code_source: 'filename',
                  },
                ],
              },
              {
                code: 'SSIS-002',
                count: 2,
                total_size: 7000000,
                reclaimable_size: 3000000,
                has_multi_part: true,
                files: [
                  {
                    relative_path: 'SSIS-002-CD1.mp4',
                    filename: 'SSIS-002-CD1.mp4',
                    size: 4000000,
                    modified_at_unix_ms: 1710000000000,
                    code: 'SSIS-002',
                    code_source: 'filename',
                    part_label: 'CD1',
                  },
                  {
                    relative_path: 'SSIS-002-CD2.mp4',
                    filename: 'SSIS-002-CD2.mp4',
                    size: 3000000,
                    modified_at_unix_ms: 1710000000000,
                    code: 'SSIS-002',
                    code_source: 'filename',
                    part_label: 'CD2',
                  },
                ],
              },
            ],
          });
        }
        if (url.pathname === '/directories/1/cleanup/delete') {
          const body = JSON.parse(init.body || '{}');
          window.cleanupRequests.push({ endpoint: 'delete', body });
          return Response.json({
            deleted_count: (body.paths || []).length,
            deleted_size: 4096,
            deleted_paths: body.paths || [],
            hidden_video_locations: 0,
            empty_directories_removed: 0,
            failed: [],
          });
        }
        return originalFetch(input, init);
      };
      document.querySelector('aside button[aria-label="Settings"]').click();
    }`)

    await waitFor(
      `Array.from(document.querySelectorAll('button')).some(b => b.textContent.includes('Directory Management'))`
    )
    await evaluate(
      `Array.from(document.querySelectorAll('button')).find(b => b.textContent.includes('Directory Management')).click()`
    )
    await waitFor(`document.querySelector('button[aria-label="Tools"]')`)
    await evaluate(`document.querySelector('button[aria-label="Tools"]').click()`)
    await waitFor(`document.getElementById('directory-process-cleanup')`)

    // Selecting Junk File Cleanup should hide organization layout choices and show Open Cleanup
    await evaluate(`document.getElementById('directory-process-cleanup').click()`)
    assert.equal(
      await evaluate(`Boolean(document.getElementById('directory-process-layout-prefix'))`),
      false
    )
    await evaluate(
      `Array.from(document.querySelectorAll('[role="dialog"] button')).find(b => b.textContent === 'Open Cleanup').click()`
    )

    // Wait for initial cleanup scan and rendered items
    await waitFor(`window.cleanupRequests.some(r => r.endpoint === 'scan')`)
    await waitFor(`document.body.textContent.includes('pack/IPX-001.torrent')`)

    // Select one junk file and confirm deletion
    await evaluate(
      `document.querySelector('input[aria-label="Select pack/IPX-001.torrent"]').click()`
    )
    await evaluate(
      `Array.from(document.querySelectorAll('[role="dialog"] button')).find(b => b.textContent.includes('Clean Selected (1)')).click()`
    )
    await waitFor(
      `Array.from(document.querySelectorAll('[role="dialog"] button')).some(b => b.textContent.includes('Confirm Delete'))`
    )
    await evaluate(
      `Array.from(document.querySelectorAll('[role="dialog"] button')).find(b => b.textContent.includes('Confirm Delete')).click()`
    )
    await waitFor(`!document.body.textContent.includes('pack/IPX-001.torrent')`)

    // Switch to Duplicates tab, use Keep Largest per Group (skips multi-part SSIS-002), and delete
    await evaluate(
      `Array.from(document.querySelectorAll('[role="tab"]')).find(b => b.textContent.includes('Duplicate Files')).click()`
    )
    await waitFor(`document.body.textContent.includes('sd/IPX-001-low.mp4')`)
    await evaluate(
      `Array.from(document.querySelectorAll('[role="dialog"] button')).find(b => b.textContent.includes('Keep Largest per Group')).click()`
    )
    await waitFor(
      `document.querySelector('input[aria-label="Select sd/IPX-001-low.mp4"]').checked === true`
    )
    assert.equal(
      await evaluate(
        `document.querySelector('input[aria-label="Select SSIS-002-CD2.mp4"]').checked`
      ),
      false,
      'multi-part duplicate group must not be auto-selected'
    )
    await evaluate(
      `Array.from(document.querySelectorAll('[role="dialog"] button')).find(b => b.textContent.includes('Delete Selected (1)')).click()`
    )
    await waitFor(
      `Array.from(document.querySelectorAll('[role="dialog"] button')).some(b => b.textContent.includes('Confirm Delete'))`
    )
    await evaluate(
      `Array.from(document.querySelectorAll('[role="dialog"] button')).find(b => b.textContent.includes('Confirm Delete')).click()`
    )
    await waitFor(`!document.body.textContent.includes('sd/IPX-001-low.mp4')`)

    const requests = await evaluate('window.cleanupRequests')
    const deletes = requests.filter((r) => r.endpoint === 'delete')
    assert.equal(deletes.length, 2)
    assert.deepEqual(deletes[0].body.paths, ['pack/IPX-001.torrent'])
    assert.deepEqual(deletes[1].body.paths, ['sd/IPX-001-low.mp4'])
    assert.deepEqual(await evaluate('window.appErrors'), [])
  }
)
