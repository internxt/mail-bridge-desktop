import { z } from 'zod'

/**
 * Add new variables here — the type and the runtime validation both derive
 * from this single source of truth.
 */
const envSchema = z.object({
  MAIN_VITE_DRIVE_API_URL: z.url(),
  MAIN_VITE_MAIL_API_URL: z.url()
})

type Env = {
  DRIVE_API_URL: string
  MAIL_API_URL: string
}

function parseEnv(): Env {
  const result = envSchema.safeParse(import.meta.env)

  if (!result.success) {
    const issues = result.error.issues
      .map((issue) => `  - ${issue.path.join('.')}: ${issue.message}`)
      .join('\n')

    throw new Error(`Invalid environment variables:\n${issues}`)
  }

  return {
    DRIVE_API_URL: result.data.MAIN_VITE_DRIVE_API_URL,
    MAIL_API_URL: result.data.MAIN_VITE_MAIL_API_URL
  }
}

export class Config {
  private static env: Env = parseEnv()

  static getVariable<K extends keyof Env>(key: K): Env[K] {
    return this.env[key]
  }
}
