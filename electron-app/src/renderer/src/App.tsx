import { useState } from 'react'
import { WindowFrame } from './components/shared/WindowFrame'
import { Sidebar } from './components/shared/Sidebar'
import { MailboxView } from './views/MailboxView'
import { SettingsView } from './views/SettingsView'
import { Onboarding } from './views/Onboarding'
import { useTheme } from './hooks/useTheme'
import type { View } from './components/shared/data'

const SYNC_PROGRESS = 4

const App = (): React.JSX.Element => {
  const { theme, resolved, setTheme } = useTheme('light')
  const [linked, setLinked] = useState(false)
  const [view, setView] = useState<View>('mailbox')

  return (
    <WindowFrame>
      {linked ? (
        <>
          <Sidebar
            view={view}
            progress={SYNC_PROGRESS}
            onShowMailbox={() => setView('mailbox')}
            onShowSettings={() => setView('settings')}
          />
          {view === 'mailbox' ? (
            <MailboxView progress={SYNC_PROGRESS} />
          ) : (
            <SettingsView
              theme={theme}
              onThemeChange={setTheme}
              onBack={() => setView('mailbox')}
            />
          )}
        </>
      ) : (
        <Onboarding
          onSignIn={() => setLinked(true)}
          onToggleTheme={() => setTheme(resolved === 'dark' ? 'light' : 'dark')}
        />
      )}
    </WindowFrame>
  )
}

export default App
