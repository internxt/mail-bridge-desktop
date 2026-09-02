import {
  ArrowSquareOutIcon,
  DownloadSimpleIcon,
  EnvelopeIcon,
  LockIcon,
  Moon,
  QuestionIcon
} from '@phosphor-icons/react'
import { useTranslation } from 'react-i18next'

interface OnboardingProps {
  onSignIn: () => void
  onToggleTheme: () => void
}

// `id` maps to onboarding.features.<id> in the locale files. Icons stay in code.
const FEATURES = [
  { id: 'local', icon: <LockIcon /> },
  { id: 'anyClient', icon: <EnvelopeIcon /> },
  { id: 'background', icon: <DownloadSimpleIcon /> }
]

export const Onboarding = ({ onSignIn, onToggleTheme }: OnboardingProps): React.JSX.Element => {
  const { t } = useTranslation()

  return (
    <>
      {/* Idle sidebar */}
      <div className="flex w-[276px] flex-none flex-col border-r border-gray-10 bg-gray-1">
        <div className="p-3.5">
          <div className="flex items-center gap-2.5 rounded-[10px] bg-gray-5 px-3 py-2.5">
            <span className="h-2 w-2 rounded-full bg-gray-20" />
            <div className="flex-1">
              <div className="text-[12.5px] font-bold text-gray-80">{t('onboarding.idle')}</div>
              <div className="text-[11.5px] text-gray-50">{t('onboarding.noMailbox')}</div>
            </div>
          </div>
        </div>
        <div className="px-4 pb-2 pt-0.5 text-[11px] font-semibold uppercase tracking-[0.09em] text-gray-50">
          {t('common.mailboxes')}
        </div>
        <div className="mx-3.5 rounded-[10px] border border-dashed border-gray-20 bg-surface px-3.5 py-4">
          <div className="text-[13px] font-semibold text-gray-100">
            {t('onboarding.empty.title')}
          </div>
          <div className="mt-1 text-[12.5px] leading-snug text-gray-50">
            {t('onboarding.empty.note')}
          </div>
        </div>
        <span className="flex-1" />
        <div className="h-px bg-gray-10" />
        <div className="flex items-center gap-1 px-3 py-2.5">
          <span className="flex-1" />
          <button
            title={t('common.help')}
            className="grid h-[30px] w-[30px] place-items-center rounded-lg text-gray-60 hover:bg-gray-5 hover:text-gray-100"
          >
            <QuestionIcon size={17} />
          </button>
          <button
            onClick={onToggleTheme}
            title={t('common.switchTheme')}
            className="grid h-[30px] w-[30px] place-items-center rounded-lg text-gray-60 hover:bg-gray-5 hover:text-gray-100"
          >
            <Moon size={17} />
          </button>
        </div>
      </div>

      {/* Hero */}
      <div className="flex min-w-0 flex-1 flex-col justify-center px-[72px]">
        <h1 className="m-0 max-w-[16ch] text-[34px] font-semibold leading-[1.12] tracking-tight text-gray-100 text-pretty">
          {t('onboarding.hero.title')}
        </h1>
        <p className="mt-3.5 max-w-[48ch] text-[15px] leading-relaxed text-gray-80 text-pretty">
          {t('onboarding.hero.subtitle')}
        </p>

        <div className="mt-7 flex items-center gap-3.5">
          <button
            onClick={onSignIn}
            className="flex h-11 items-center gap-2.5 rounded-lg bg-primary px-[18px] text-sm font-semibold text-white hover:bg-primary-dark"
          >
            <ArrowSquareOutIcon size={16} weight="bold" />
            {t('onboarding.hero.signIn')}
          </button>
          <a className="text-[13.5px] font-semibold text-primary hover:underline" href="#">
            {t('onboarding.hero.howItWorks')}
          </a>
        </div>

        <div className="mt-[38px] grid max-w-[620px] grid-cols-3 gap-3">
          {FEATURES.map((f) => (
            <div key={f.id} className="rounded-[10px] border border-gray-10 bg-gray-5 p-3.5">
              <span className="text-primary">{f.icon}</span>
              <div className="mt-2.5 text-[12.5px] font-semibold text-gray-100">
                {t(`onboarding.features.${f.id}.title`)}
              </div>
              <div className="mt-[3px] text-xs leading-snug text-gray-60">
                {t(`onboarding.features.${f.id}.note`)}
              </div>
            </div>
          ))}
        </div>
      </div>
    </>
  )
}
