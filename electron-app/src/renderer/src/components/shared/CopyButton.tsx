import { CheckIcon, CopyIcon } from '@phosphor-icons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

export const CopyButton = ({ value }: { value: string }): React.JSX.Element => {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const onCopy = (): void => {
    void navigator.clipboard?.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1400)
  }

  return (
    <button
      onClick={onCopy}
      title={copied ? t('common.copied') : t('common.copy')}
      className={`grid h-7 w-7 flex-none place-items-center rounded-md hover:bg-gray-5 ${
        copied ? 'text-green' : 'text-gray-40'
      }`}
    >
      {copied ? <CheckIcon size={15} weight="bold" /> : <CopyIcon size={15} />}
    </button>
  )
}
