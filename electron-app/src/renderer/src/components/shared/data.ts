// Static content for the Mail Bridge UI, mirroring the design's DCLogic data.
// Kept separate from components so the views stay presentational.
//
// This file holds only MOCK data and structure — no translatable copy. Any
// user-facing text (labels, notes, descriptions) is resolved in the components
// via i18n keys. See src/renderer/src/i18n/locales/*.json.

export type ThemeId = 'light' | 'dark' | 'system'
export type View = 'mailbox' | 'settings'

export interface MailClient {
  name: string
  note: string
  initial: string
  translate?: string
}

/** A connection field. `labelKey` is an i18n key; `value` is mock data. */
export interface ConnectionRow {
  labelKey: 'hostname' | 'port' | 'username' | 'password' | 'security'
  value: string
}

export interface ThemeOption {
  id: ThemeId
  preview: { app: string; sidebar: string; rule: string; accent: string }
}

export interface Preference {
  key: 'login' | 'menubar' | 'updates'
}

export const MAIL_CLIENTS: MailClient[] = [
  { name: 'Apple Mail', note: 'macOS', initial: 'A' },
  { name: 'Outlook', note: '365 / 2021', initial: 'O' },
  { name: 'Thunderbird', note: '115+', initial: 'T' },
  { name: 'Other client', note: 'manual setup', initial: '+', translate: 'other' }
]

export const THEME_OPTIONS: ThemeOption[] = [
  {
    id: 'light',
    preview: { app: '#ffffff', sidebar: '#F1F2F6', rule: '#DDE0E6', accent: '#0066FF' }
  },
  {
    id: 'dark',
    preview: { app: '#1E2026', sidebar: '#16171B', rule: '#3A3D45', accent: '#2E7DFF' }
  },
  {
    id: 'system',
    preview: { app: '#ffffff', sidebar: '#16171B', rule: '#9BA1AD', accent: '#0066FF' }
  }
]

export const PREFERENCES: Preference[] = [{ key: 'login' }, { key: 'menubar' }, { key: 'updates' }]

const MASKED_PASSWORD = '••••••••••••'
const REVEALED_PASSWORD = 'z8Tq-4Kd1-Vm9c'

/** IMAP/SMTP connection rows. `showPw` reveals the password value. */
export function connectionRows(
  kind: 'imap' | 'smtp',
  username: string,
  showPw: boolean
): ConnectionRow[] {
  return [
    { labelKey: 'hostname', value: '127.0.0.1' },
    { labelKey: 'port', value: kind === 'imap' ? '1143' : '1025' },
    { labelKey: 'username', value: username },
    { labelKey: 'password', value: showPw ? REVEALED_PASSWORD : MASKED_PASSWORD },
    { labelKey: 'security', value: kind === 'imap' ? 'STARTTLS' : 'SSL' }
  ]
}
