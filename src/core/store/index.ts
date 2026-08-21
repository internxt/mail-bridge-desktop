import ElectronStore, { Schema } from 'electron-store'
import { Logger } from '../logger'
import { AppStore } from './types'
import { safeStorage } from 'electron'

const TOKEN_ENCODING = 'latin1'

const logger = new Logger('Store')

const defaults: AppStore = {
  theme: 'light',
  locale: 'en'
}

const schema: Schema<AppStore> = {
  theme: { type: 'string', default: defaults.theme, enum: ['light', 'dark', 'system'] },
  locale: { type: 'string', default: defaults.locale },
  user: { type: 'object', default: defaults.user },
  token: { type: 'string', default: defaults.token }
}

export class Store {
  static readonly instance: Store = new Store()
  private readonly store: ElectronStore<AppStore>

  constructor() {
    this.store = new ElectronStore<AppStore>({ schema, defaults })
  }

  isLoggedIn(): boolean {
    try {
      const user = this.getUser()
      const token = this.getToken()

      return !!user && !!token
    } catch (error) {
      logger.error({ msg: 'Failed to resolve auth state', error })
      return false
    }
  }

  getToken() {
    const storedToken = this.store.get('token')
    if (!storedToken) return
    return safeStorage.decryptString(Buffer.from(storedToken, TOKEN_ENCODING))
  }

  setToken(token: string) {
    const buffer = safeStorage.encryptString(token)

    const encryptedToken = buffer.toString(TOKEN_ENCODING)
    this.store.set('token', encryptedToken)
  }

  getUser() {
    return this.store.get('user')
  }

  setUser(user: AppStore['user']) {
    this.store.set('user', user)
  }

  get<K extends keyof AppStore>(key: K): AppStore[K] {
    return this.store.get(key)
  }

  set<K extends keyof AppStore>(key: K, value: AppStore[K]) {
    logger.debug({ msg: 'Setting store key', key })
    this.store.set(key, value)
  }

  delete<K extends keyof AppStore>(key: K) {
    logger.debug({ msg: 'Deleting store key', key })
    this.store.delete(key)
  }

  clear() {
    logger.debug({ msg: 'Clearing store' })
    this.store.clear()
  }
}
