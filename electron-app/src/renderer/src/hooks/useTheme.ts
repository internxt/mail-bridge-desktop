import { useEffect, useState } from 'react'
import type { ThemeId } from '../components/shared/data'

export function useTheme(initial: ThemeId = 'light'): {
  theme: ThemeId
  resolved: 'light' | 'dark'
  setTheme: (t: ThemeId) => void
} {
  const [theme, setTheme] = useState<ThemeId>(initial)
  const [systemDark, setSystemDark] = useState(
    () => window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false
  )

  useEffect(() => {
    const preferredColor = window.matchMedia?.('(prefers-color-scheme: dark)')
    if (!preferredColor) return
    const onChange = (e: MediaQueryListEvent): void => setSystemDark(e.matches)
    preferredColor.addEventListener('change', onChange)
    return () => preferredColor.removeEventListener('change', onChange)
  }, [])

  const resolved: 'light' | 'dark' = theme === 'system' ? (systemDark ? 'dark' : 'light') : theme

  useEffect(() => {
    document.documentElement.classList.toggle('dark', resolved === 'dark')
  }, [resolved])

  return { theme, resolved, setTheme }
}
