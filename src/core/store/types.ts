import { UserData } from '../../types'

export type Theme = 'light' | 'dark' | 'system'

export interface AppStore {
  theme: Theme
  locale: string
  user?: UserData
  token?: string
}
