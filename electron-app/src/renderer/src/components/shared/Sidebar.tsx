import { GearIcon, PlusIcon } from '@phosphor-icons/react'
import { useTranslation } from 'react-i18next'
import type { View } from './data'

interface SidebarProps {
  view: View
  progress: number
  onShowMailbox: () => void
  onShowSettings: () => void
}

export function Sidebar({
  view,
  progress,
  onShowMailbox,
  onShowSettings
}: SidebarProps): React.JSX.Element {
  const { t } = useTranslation()

  return (
    <div className="flex w-[276px] flex-none flex-col border-r border-gray-10 bg-gray-1">
      {/* Running status */}
      <div className="p-3.5 pb-3">
        <div className="flex items-center gap-2.5 rounded-[10px] bg-primary px-3 py-2.5 text-white">
          <span className="h-2 w-2 flex-none rounded-full bg-green shadow-[0_0_0_3px_rgba(127,227,167,0.25)]" />
          <div className="min-w-0 flex-1">
            <div className="text-[12.5px] font-bold tracking-tight">{t('sidebar.running')}</div>
            <div className="text-[11.5px] opacity-85">127.0.0.1 · ports 1143 / 1025</div>
          </div>
        </div>
      </div>

      {/* Mailboxes header */}
      <div className="flex items-center gap-2 px-4 pb-2 pt-1.5">
        <span className="text-[11px] font-semibold uppercase tracking-[0.09em] text-gray-50">
          {t('common.mailboxes')}
        </span>
        <span className="h-px flex-1 bg-gray-10" />
        <span className="text-[11px] font-semibold text-gray-50">1</span>
      </div>

      {/* Mailbox list */}
      <div className="flex flex-col gap-0.5 px-2.5">
        <button
          onClick={onShowMailbox}
          className={`flex items-center gap-2.5 rounded-lg border px-2.5 py-2.5 text-left ${
            view === 'mailbox'
              ? 'border-primary/30 bg-surface'
              : 'border-transparent hover:bg-gray-5'
          }`}
        >
          <div className="grid h-8 w-8 flex-none place-items-center rounded-lg bg-primary text-sm font-semibold text-white">
            M
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-[13px] font-semibold text-gray-100">marta@inxt.com</div>
            <div className="mt-0.5 flex items-center gap-1.5">
              <span className="h-[3px] w-[54px] overflow-hidden rounded-full bg-gray-10">
                <span
                  className="block h-full rounded-full bg-primary"
                  style={{ width: `${progress}%` }}
                />
              </span>
              <span className="text-[11.5px] font-semibold text-primary">{progress}%</span>
            </div>
          </div>
        </button>
      </div>

      <span className="flex-1" />

      <div className="h-px bg-gray-10" />

      {/* Footer actions */}
      <div className="flex items-center gap-1 px-3 py-2.5">
        <button className="flex items-center gap-2 rounded-lg px-2.5 py-2 text-[13px] font-semibold text-primary hover:bg-primary/10">
          <PlusIcon size={16} weight="bold" />
          {t('sidebar.addMailbox')}
        </button>
        <span className="flex-1" />
        <button
          onClick={onShowSettings}
          title={t('common.settings')}
          className={`grid h-[30px] w-[30px] place-items-center rounded-lg hover:bg-gray-5 ${
            view === 'settings' ? 'bg-gray-5 text-gray-100' : 'text-gray-60'
          }`}
        >
          <GearIcon size={17} />
        </button>
      </div>
    </div>
  )
}
