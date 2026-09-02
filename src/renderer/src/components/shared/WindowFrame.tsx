import type { ReactNode } from 'react'

/**
 * the app body. Shared by both the onboarding and the main states.
 */
export const WindowFrame = ({ children }: { children: ReactNode }): React.JSX.Element => {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-surface text-gray-100">
      <div className="flex min-h-0 flex-1">{children}</div>
    </div>
  )
}
