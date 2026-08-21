import { app } from 'electron'

type LogLevel = 'debug' | 'info' | 'warn' | 'error'

type LogPayload = {
  msg: string
  [key: string]: unknown
}

export class Logger {
  private readonly service: string

  constructor(service: string) {
    this.service = service
  }

  debug(payload: LogPayload) {
    this.log('debug', payload)
  }

  info(payload: LogPayload) {
    this.log('info', payload)
  }

  warn(payload: LogPayload) {
    this.log('warn', payload)
  }

  error(payload: LogPayload) {
    this.log('error', payload)
  }

  private log(level: LogLevel, { msg, ...params }: LogPayload) {
    const entry = {
      timestamp: new Date().toISOString(),
      version: this.version,
      service: this.service,
      level,
      msg,
      ...params
    }

    console[level](JSON.stringify(entry))
  }

  private get version() {
    return app?.isReady() ? app.getVersion() : (process.env.npm_package_version ?? 'unknown')
  }
}
