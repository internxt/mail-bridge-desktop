import { useState } from 'react'
import { ArrowLeft } from '@phosphor-icons/react'
import { useTranslation } from 'react-i18next'
import {
  PREFERENCES,
  THEME_OPTIONS,
  type Preference,
  type ThemeId
} from '../components/shared/data'

interface SettingsViewProps {
  theme: ThemeId
  onThemeChange: (t: ThemeId) => void
  onBack: () => void
}

const BRIDGE_VERSION = '1.4.2'

export const SettingsView = ({
  theme,
  onThemeChange,
  onBack
}: SettingsViewProps): React.JSX.Element => {
  const { t } = useTranslation()
  const [prefs, setPrefs] = useState<Record<Preference['key'], boolean>>({
    login: true,
    menubar: true,
    updates: false
  })

  return (
    <div className="flex min-w-0 flex-1 flex-col overflow-auto">
      {/* Header */}
      <div className="flex items-center gap-3 px-7 pb-[18px] pt-[22px]">
        <button
          onClick={onBack}
          className="grid h-8 w-8 place-items-center rounded-lg border border-gray-20 bg-surface text-gray-80 hover:bg-gray-5 hover:text-gray-100"
        >
          <ArrowLeft size={15} weight="bold" />
        </button>
        <div>
          <div className="text-[11px] font-bold uppercase tracking-[0.09em] text-gray-50">
            {t('settings.preferences')}
          </div>
          <div className="mt-[3px] text-[23px] font-semibold tracking-tight text-gray-100">
            {t('common.settings')}
          </div>
        </div>
      </div>

      {/* Appearance */}
      <div className="px-7 pb-[22px]">
        <div className="text-sm font-semibold text-gray-100">{t('settings.appearance.title')}</div>
        <div className="mt-[3px] text-[13px] text-gray-60">{t('settings.appearance.note')}</div>

        <div className="mt-3.5 grid grid-cols-3 gap-3">
          {THEME_OPTIONS.map((option) => {
            const on = theme === option.id
            return (
              <button
                key={option.id}
                onClick={() => onThemeChange(option.id)}
                className={`rounded-xl border p-3 text-left ${
                  on
                    ? 'border-primary bg-primary/[0.06]'
                    : 'border-gray-10 bg-surface hover:bg-gray-5'
                }`}
              >
                <span
                  className="block h-[62px] overflow-hidden rounded-lg border border-gray-10"
                  style={{ background: option.preview.app }}
                >
                  <span className="flex h-full">
                    <span
                      className="w-[34%] border-r"
                      style={{
                        background: option.preview.sidebar,
                        borderColor: option.preview.rule
                      }}
                    />
                    <span className="block flex-1 p-2">
                      <span
                        className="block h-1.5 w-[60%] rounded-full"
                        style={{ background: option.preview.accent }}
                      />
                      <span
                        className="mt-1.5 block h-[5px] w-[85%] rounded-full"
                        style={{ background: option.preview.rule }}
                      />
                      <span
                        className="mt-[5px] block h-[5px] w-[70%] rounded-full"
                        style={{ background: option.preview.rule }}
                      />
                    </span>
                  </span>
                </span>
                <span className="mt-2.5 flex items-center gap-2">
                  <span
                    className={`grid h-4 w-4 flex-none place-items-center rounded-full border ${
                      on ? 'border-primary bg-primary' : 'border-gray-20'
                    }`}
                  >
                    {on && <span className="h-1.5 w-1.5 rounded-full bg-white" />}
                  </span>
                  <span className="text-[13px] font-semibold text-gray-100">
                    {t(`settings.themes.${option.id}.label`)}
                  </span>
                </span>
                <span className="mt-1 block pl-6 text-[11.5px] text-gray-60">
                  {t(`settings.themes.${option.id}.note`)}
                </span>
              </button>
            )
          })}
        </div>
      </div>

      <div className="mx-7 h-px bg-gray-10" />

      {/* Behaviour */}
      <div className="px-7 pb-1.5 pt-[18px] text-sm font-semibold text-gray-100">
        {t('settings.behaviour.title')}
      </div>
      <div className="px-7 pb-[18px]">
        {PREFERENCES.map((p) => {
          const on = prefs[p.key]
          return (
            <div
              key={p.key}
              className="flex items-center gap-4 border-b border-gray-5 py-3.5 last:border-0"
            >
              <div className="flex-1">
                <div className="text-[13px] font-semibold text-gray-100">
                  {t(`settings.behaviour.${p.key}.title`)}
                </div>
                <div className="mt-0.5 text-[12.5px] text-gray-60">
                  {t(`settings.behaviour.${p.key}.note`)}
                </div>
              </div>
              <button
                onClick={() => setPrefs((s) => ({ ...s, [p.key]: !s[p.key] }))}
                className={`flex h-6 w-10 flex-none rounded-full p-[3px] ${
                  on ? 'justify-end bg-primary' : 'justify-start bg-gray-20'
                }`}
              >
                <span className="h-[18px] w-[18px] rounded-full bg-white shadow-sm" />
              </button>
            </div>
          )
        })}
      </div>

      <div className="mx-7 h-px bg-gray-10" />

      {/* Local ports */}
      <div className="flex items-end gap-[18px] px-7 pb-[26px] pt-[18px]">
        <div className="flex-1">
          <div className="text-sm font-semibold text-gray-100">{t('settings.ports.title')}</div>
          <div className="mt-[3px] text-[13px] text-gray-60">{t('settings.ports.note')}</div>
          <div className="mt-3 flex gap-3">
            <PortField label="IMAP" defaultValue="1143" />
            <PortField label="SMTP" defaultValue="1025" />
          </div>
        </div>
        <div className="flex-none text-right text-xs leading-relaxed text-gray-50">
          <div>{t('settings.version', { version: BRIDGE_VERSION })}</div>
          <div className="mt-1.5 flex justify-end gap-3">
            <a className="font-semibold text-primary hover:underline" href="#">
              {t('settings.viewLogs')}
            </a>
            <a className="font-semibold text-primary hover:underline" href="#">
              {t('settings.reportProblem')}
            </a>
          </div>
        </div>
      </div>
    </div>
  )
}

const PortField = ({
  label,
  defaultValue
}: {
  label: string
  defaultValue: string
}): React.JSX.Element => {
  return (
    <label className="block">
      <span className="block text-[11.5px] font-semibold text-gray-60">{label}</span>
      <input
        defaultValue={defaultValue}
        className="mt-[5px] h-[34px] w-[110px] rounded-lg border border-gray-20 bg-surface px-2.5 text-[13px] tabular-nums text-gray-100 outline-none focus:border-primary"
      />
    </label>
  )
}
