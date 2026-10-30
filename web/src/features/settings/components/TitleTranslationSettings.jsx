import { useState } from 'react'
import { useStore } from '@/store'
import { saveTitleTranslationSettings } from '@/features/settings/actions'
import { configFlag } from '@/utils/config'
import { getErrorMessage } from '@/utils/errors'
import { zh } from '@/utils/i18n'

export default function TitleTranslationSettings() {
  const config = useStore((state) => state.config)
  const configured = configFlag(config?.deepseek_api_key_configured)
  const [apiKey, setApiKey] = useState('')
  const [model, setModel] = useState(config?.deepseek_model || 'deepseek-flash')
  const [autoTranslate, setAutoTranslate] = useState(configFlag(config?.deepseek_auto_translate))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const save = async (clear = false) => {
    if (saving) return
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      const updated = await saveTitleTranslationSettings(
        clear
          ? { deepseek_api_key: '', deepseek_auto_translate: false }
          : {
              ...(apiKey.trim() ? { deepseek_api_key: apiKey.trim() } : {}),
              deepseek_model: model.trim() || 'deepseek-flash',
              deepseek_auto_translate: autoTranslate,
            }
      )
      setApiKey('')
      setModel(updated.deepseek_model || 'deepseek-flash')
      setAutoTranslate(configFlag(updated.deepseek_auto_translate))
      setSuccess(zh('翻译设置已保存', 'Translation settings saved'))
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  const inputClass =
    'mt-2 w-full rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-900 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-100'

  return (
    <form
      className="max-w-2xl space-y-5"
      onSubmit={(event) => {
        event.preventDefault()
        void save()
      }}
    >
      <div>
        <h3 className="text-base font-semibold text-gray-900">
          {zh('DeepSeek 标题翻译', 'DeepSeek title translation')}
        </h3>
        <p className="mt-2 text-sm text-gray-600">
          {zh(
            '将作品原始标题翻译为简体中文。存在中文标题时优先展示，原始标题保留。',
            'Translate original titles into Simplified Chinese. Chinese titles are shown first, while originals are retained.'
          )}
        </p>
      </div>
      <div>
        <label htmlFor="deepseek-api-key" className="text-sm font-medium text-gray-900">
          DeepSeek API Key
        </label>
        <input
          id="deepseek-api-key"
          type="password"
          autoComplete="new-password"
          value={apiKey}
          onChange={(event) => setApiKey(event.target.value)}
          placeholder={
            configured ? zh('已配置，留空保留', 'Configured; leave blank to keep') : 'sk-…'
          }
          aria-describedby="deepseek-api-key-hint"
          maxLength={512}
          disabled={saving}
          className={inputClass}
        />
        <p id="deepseek-api-key-hint" className="mt-1 text-xs text-gray-500">
          {zh(
            '密钥保存在服务端，保存后不回显。',
            'The key is saved on the server and never returned.'
          )}{' '}
          <a
            href="https://platform.deepseek.com/api_keys"
            target="_blank"
            rel="noreferrer"
            className="text-blue-600 underline"
          >
            {zh('获取 API Key', 'Get an API key')}
          </a>
        </p>
      </div>
      <div>
        <label htmlFor="deepseek-model" className="text-sm font-medium text-gray-900">
          {zh('模型', 'Model')}
        </label>
        <input
          id="deepseek-model"
          value={model}
          onChange={(event) => setModel(event.target.value)}
          placeholder="deepseek-flash"
          maxLength={128}
          disabled={saving}
          className={inputClass}
        />
      </div>
      <label className="flex items-center gap-3 text-sm font-medium text-gray-900">
        <input
          type="checkbox"
          checked={autoTranslate}
          onChange={(event) => setAutoTranslate(event.target.checked)}
          disabled={saving}
          className="h-4 w-4 rounded border-gray-300 accent-blue-600"
        />
        {zh('自动补齐中文标题', 'Automatically fill missing Chinese titles')}
      </label>
      <p className="text-sm text-gray-600">
        {zh(
          '启用后，每分钟分批翻译现有及新增作品中缺失的中文标题，已有中文标题会保留。原始标题会发送至 DeepSeek，并按其 API 用量计费。也可在作品编辑窗口手动翻译。',
          'When enabled, missing Chinese titles in existing and new works are translated in batches every minute. Existing Chinese titles are preserved. Original titles are sent to DeepSeek and billed at its API rates. You can also translate manually in the work editor.'
        )}
      </p>
      {error && (
        <p role="alert" className="text-sm text-red-600">
          {error}
        </p>
      )}
      {success && (
        <p role="status" className="text-sm text-green-700">
          {success}
        </p>
      )}
      <div className="flex flex-wrap gap-3">
        <button
          type="submit"
          disabled={saving || (autoTranslate && !configured && !apiKey.trim())}
          className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {saving ? zh('保存中…', 'Saving…') : zh('保存', 'Save')}
        </button>
        {configured && (
          <button
            type="button"
            disabled={saving}
            onClick={() => void save(true)}
            className="rounded-md border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-50"
          >
            {zh('清除 API Key', 'Clear API key')}
          </button>
        )}
      </div>
    </form>
  )
}
