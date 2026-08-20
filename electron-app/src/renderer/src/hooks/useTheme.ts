import { useEffect, useState } from 'react'
import type { ThemeId } from '../components/shared/data'

/**
 * Manages the light/dark/system appearance and keeps the `.dark` class on
 * <html> in sync (class-based dark mode, matching css-config's darkMode:"class").
 * "system" follows the OS via matchMedia and updates live.
 */
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
    const mq = window.matchMedia?.('(prefers-color-scheme: dark)')
    if (!mq) return
    const onChange = (e: MediaQueryListEvent): void => setSystemDark(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const resolved: 'light' | 'dark' =
    theme === 'system' ? (systemDark ? 'dark' : 'light') : theme

  useEffect(() => {
    document.documentElement.classList.toggle('dark', resolved === 'dark')
  }, [resolved])

  return { theme, resolved, setTheme }
}
