import { Navigate, Outlet } from 'react-router-dom'
import { useAuth } from './AuthContext'
import { DEFAULT_PRIVATE_ROUTE, DEFAULT_PUBLIC_ROUTE } from './routes'

export const ProtectedRoute = () => {
  const { isLoggedIn } = useAuth()
  return isLoggedIn ? <Outlet /> : <Navigate to={DEFAULT_PUBLIC_ROUTE} replace />
}

export const PublicRoute = () => {
  const { isLoggedIn } = useAuth()
  return isLoggedIn ? <Navigate to={DEFAULT_PRIVATE_ROUTE} replace /> : <Outlet />
}
