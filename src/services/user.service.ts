import { SdkManager } from '../core/sdk'

export class UserService {
  static readonly instance: UserService = new UserService()

  private get users() {
    return SdkManager.instance.getUsers()
  }

  async refreshUser() {
    const refreshedUser = await this.users.refreshUser()

    return refreshedUser
  }
}
