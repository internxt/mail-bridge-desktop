import { Logger } from '../core/logger'
import { SdkManager } from '../core/sdk'

export class AuthService {
  private readonly logger = new Logger('Auth')
  static readonly instance: AuthService = new AuthService()

  private get auth() {
    return SdkManager.instance.getAuth()
  }

  async logout() {
    try {
      await this.auth.logout()
    } catch (error) {
      this.logger.error({ msg: 'Error logging out', error })
    }
  }
}
