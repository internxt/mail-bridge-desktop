import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConnectionRow, connectionRows } from '../shared/data'
import { TabButton } from './TabButton'
import { CaretDownIcon, CaretUpIcon } from '@phosphor-icons/react'
import { CopyButton } from '../shared/CopyButton'

const USERNAME = 'marta@inxt.com'
type ConnectionKind = 'imap' | 'smtp'

export const ConnectionPanel = ({ showPw }: { showPw: boolean }): React.JSX.Element => {
  const { t } = useTranslation()
  const [kind, setKind] = useState<ConnectionKind>('imap')
  const rows: ConnectionRow[] = connectionRows(kind, USERNAME, showPw)

  return (
    <div className="overflow-hidden rounded-xl border border-gray-10 bg-surface">
      {/* Tabs */}
      <div className="flex items-center gap-1 border-b border-gray-10 bg-gray-1 p-1.5">
        <TabButton
          active={kind === 'imap'}
          onClick={() => setKind('imap')}
          icon={<CaretDownIcon size={14} weight="bold" />}
          label="IMAP"
          note={t('mailbox.connection.incoming')}
        />
        <TabButton
          active={kind === 'smtp'}
          onClick={() => setKind('smtp')}
          icon={<CaretUpIcon size={14} weight="bold" />}
          label="SMTP"
          note={t('mailbox.connection.outgoing')}
        />
      </div>

      {/* Rows — fixed height regardless of value length */}
      <div className="px-4 py-1.5">
        {rows.map((row) => (
          <div
            key={row.labelKey}
            className="flex h-11 items-center gap-3 border-b border-gray-5 last:border-0"
          >
            <div className="w-[84px] flex-none text-xs font-semibold text-gray-60">
              {t(`mailbox.connection.${row.labelKey}`)}
            </div>
            <div className="min-w-0 flex-1 truncate text-[13.5px] tabular-nums text-gray-100">
              {row.value}
            </div>
            <CopyButton value={row.value} />
          </div>
        ))}
      </div>
    </div>
  )
}
