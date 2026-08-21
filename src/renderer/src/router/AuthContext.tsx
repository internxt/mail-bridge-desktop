import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'

interface AuthContextValue {
  isLoggedIn: boolean
  signIn: () => void
  signOut: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  // TODO: wire this to the real authentication
  const [isLoggedIn, setIsLoggedIn] = useState(false)

  const value = useMemo<AuthContextValue>(
    () => ({
      isLoggedIn,
      signIn: () => setIsLoggedIn(true),
      signOut: () => setIsLoggedIn(false)
    }),
    [isLoggedIn]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export const useAuth = (): AuthContextValue => {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth must be used within an AuthProvider')
  return context
}
