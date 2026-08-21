import { Auth, Drive, MailApi } from '@internxt/sdk'
import { HttpClient, type ApiSecurity, type AppDetails } from '@internxt/sdk/dist/shared'
import packageJson from '../../../package.json'
import { Config } from '../config'
import { Store } from '../store'

export type SdkManagerApiSecurity = ApiSecurity & { newToken: string }

HttpClient.enableGlobalRetry()

export class SdkManager {
  public static readonly instance: SdkManager = new SdkManager()

  private getNewTokenApiSecurity(unauthorizedCallback?: () => void): ApiSecurity {
    return {
      token: Store.instance?.getToken() ?? '',
      unauthorizedCallback:
        unauthorizedCallback ??
        (() => {
          Store.instance.clear()
        })
    }
  }

  public static readonly getAppDetails = (): AppDetails => {
    return {
      clientName: packageJson.name,
      clientVersion: packageJson.version
    }
  }

  getAuth(unauthorizedCallback?: () => void): Auth {
    const apiSecurity = this.getNewTokenApiSecurity(unauthorizedCallback)
    const appDetails = SdkManager.getAppDetails()

    return Auth.client(this.driveApiUrl, appDetails, apiSecurity)
  }

  getUsers(): Drive.Users {
    const apiSecurity = this.getNewTokenApiSecurity()
    const appDetails = SdkManager.getAppDetails()

    return Drive.Users.client(this.driveApiUrl, appDetails, apiSecurity)
  }

  getMail(): MailApi {
    const apiSecurity = this.getNewTokenApiSecurity()
    const appDetails = SdkManager.getAppDetails()

    return MailApi.client(this.mailApiUrl, appDetails, apiSecurity)
  }

  get driveApiUrl(): string {
    return Config.getVariable('DRIVE_API_URL')
  }

  get mailApiUrl(): string {
    return Config.getVariable('MAIL_API_URL')
  }
}
