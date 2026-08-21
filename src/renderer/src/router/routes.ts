export const Routes = {
  Onboarding: '/',
  Mailbox: '/mailbox',
  Settings: '/settings'
} as const

export type RoutePath = (typeof Routes)[keyof typeof Routes]

export const DEFAULT_PRIVATE_ROUTE = Routes.Mailbox
export const DEFAULT_PUBLIC_ROUTE = Routes.Onboarding
