import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwise,
  EyeIcon,
  LightningIcon,
  TrashIcon,
  TreeStructureIcon
} from '@phosphor-icons/react'
import { MAIL_CLIENTS, type MailClient } from '../components/shared/data'
import { VaultIcon } from '../components/icons/VaultIcon'
import { FlowDashes } from '../components/icons/FlowDashes'
import { MailIcon } from '../components/icons/MailIcon'
import { FlowNode } from '../components/shared/FlowNode'
import { ConnectionPanel } from '@renderer/components/mailbox/ConnectionPannel'

const USERNAME = 'marta@inxt.com'

export const MailboxView = ({ syncPct }: { syncPct: number }): React.JSX.Element => {
  const { t } = useTranslation()
  const [showPw, setShowPw] = useState(false)
  const [client, setClient] = useState('Apple Mail')

  // Resolve a client's display name/note: proper-name clients use their mock
  // strings; the "other" entry is translated via clients.<key>.
  const clientName = (c: MailClient): string => (c.translate ? t(`clients.${c.translate}.name`) : c.name)
  const clientNote = (c: MailClient): string => (c.translate ? t(`clients.${c.translate}.note`) : c.note)

  const selectedClient = MAIL_CLIENTS.find((c) => c.name === client)
  const selectedClientName = selectedClient ? clientName(selectedClient) : client

  return (
    <div className="flex min-w-0 flex-1 flex-col overflow-auto">
      {/* Header */}
      <div className="flex items-center gap-4 px-7 pb-[18px] pt-[22px]">
        <div className="min-w-0 flex-1">
          <div className="text-[11px] font-bold uppercase tracking-[0.09em] text-gray-50">
            {t('mailbox.label')}
          </div>
          <div className="mt-1 text-[23px] font-semibold tracking-tight text-gray-100">
            {USERNAME}
          </div>
        </div>
        <div className="flex flex-none gap-2">
          <button className="flex h-[34px] items-center gap-[7px] rounded-lg border border-gray-20 bg-surface px-3.5 text-[12.5px] font-semibold text-gray-100 hover:bg-gray-5">
            <ArrowsClockwise size={14} weight="bold" />
            {t('mailbox.resync')}
          </button>
          <button className="h-[34px] rounded-lg border border-gray-20 bg-surface px-3.5 text-[12.5px] font-semibold text-gray-100 hover:bg-gray-5">
            {t('mailbox.signOut')}
          </button>
          <button className="grid h-[34px] w-[34px] place-items-center rounded-lg border border-gray-20 bg-surface text-gray-60 hover:border-red/40 hover:bg-red/10 hover:text-red">
            <TrashIcon size={15} />
          </button>
        </div>
      </div>

      {/* Bridge flow card */}
      <div className="mx-7 rounded-xl border border-primary/20 bg-primary/[0.06] px-5 py-[18px]">
        <div className="flex items-center gap-3.5">
          <FlowNode
            icon={<VaultIcon />}
            title="Internxt"
            subtitle={t('mailbox.flow.vault')}
            tone="surface"
          />
          <FlowDashes />
          <FlowNode
            icon={<TreeStructureIcon size={17} weight="bold" />}
            title="Bridge"
            subtitle={t('mailbox.flow.bridge')}
            tone="accent"
          />
          <FlowDashes delay />
          <FlowNode
            icon={<MailIcon />}
            title="Apple Mail"
            subtitle={t('mailbox.flow.client')}
            tone="surface"
          />
        </div>

        <div className="mt-4 flex items-center gap-3 border-t border-primary/20 pt-3.5">
          <div className="flex-1">
            <div className="text-[12.5px] font-semibold text-gray-100">
              {t('mailbox.syncing', { pct: syncPct, done: '2,481', total: '61,904' })}
            </div>
            <div className="mt-[7px] h-[5px] overflow-hidden rounded-full bg-primary/20">
              <div className="h-full rounded-full bg-primary" style={{ width: `${syncPct}%` }} />
            </div>
          </div>
          <span className="flex-none text-xs text-gray-60">{t('mailbox.timeLeft', { min: 4 })}</span>
        </div>
      </div>

      {/* Connect a mail client */}
      <div className="flex items-end gap-4 px-7 pb-2.5 pt-[22px]">
        <div className="flex-1">
          <div className="text-[15px] font-semibold text-gray-100">
            {t('mailbox.connect.title')}
          </div>
          <div className="mt-[3px] text-[13px] text-gray-60">{t('mailbox.connect.note')}</div>
        </div>
      </div>

      {/* Client grid */}
      <div className="grid grid-cols-4 gap-2.5 px-7 pt-2">
        {MAIL_CLIENTS.map((c) => {
          const on = client === c.name
          return (
            <button
              key={c.name}
              onClick={() => setClient(c.name)}
              className={`flex items-center gap-2.5 rounded-[10px] border p-3 text-left ${
                on
                  ? 'border-primary bg-primary/[0.06]'
                  : 'border-gray-10 bg-surface hover:bg-gray-5'
              }`}
            >
              <span className="grid h-[26px] w-[26px] flex-none place-items-center rounded-[7px] border border-gray-20 bg-surface text-xs font-bold text-primary">
                {c.initial}
              </span>
              <span className="min-w-0">
                <span className="block truncate text-[12.5px] font-semibold text-gray-100">
                  {clientName(c)}
                </span>
                <span className="block text-[11.5px] text-gray-60">{clientNote(c)}</span>
              </span>
            </button>
          )
        })}
      </div>

      {/* CTA */}
      <div className="px-7 pt-3.5">
        <button className="flex h-10 items-center gap-2.5 rounded-lg bg-primary px-4 text-[13.5px] font-semibold text-white hover:bg-primary-dark">
          <LightningIcon size={16} weight="fill" />
          {t('mailbox.setup', { client: selectedClientName })}
        </button>
      </div>

      {/* Manual settings */}
      <div className="mt-[22px] border-t border-gray-10 bg-gray-5 px-7 pb-[26px] pt-5">
        <div className="mb-3.5 flex items-center gap-3">
          <div className="text-sm font-semibold text-gray-100">{t('mailbox.manual.title')}</div>
          <span className="rounded bg-primary/10 px-[7px] py-[3px] text-[11px] font-semibold uppercase tracking-[0.04em] text-primary">
            {t('mailbox.manual.badge')}
          </span>
          <span className="flex-1" />
          <button
            onClick={() => setShowPw((s) => !s)}
            className="flex h-[30px] items-center gap-[7px] rounded-lg border border-gray-20 bg-surface px-[11px] text-[12.5px] font-semibold text-gray-100 hover:bg-gray-5"
          >
            <EyeIcon size={14} />
            {showPw ? t('mailbox.manual.hidePassword') : t('mailbox.manual.showPassword')}
          </button>
        </div>

        <ConnectionPanel showPw={showPw} />
      </div>
    </div>
  )
}
