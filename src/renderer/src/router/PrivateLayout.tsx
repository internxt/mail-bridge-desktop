import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { Sidebar } from '../components/shared/Sidebar'
import { Routes } from './routes'
import type { View } from '../components/shared/data'

const SYNC_PROGRESS = 4

export const PrivateLayout = () => {
  const navigate = useNavigate()
  const { pathname } = useLocation()

  const view: View = pathname === Routes.Settings ? 'settings' : 'mailbox'

  return (
    <>
      <Sidebar
        view={view}
        progress={SYNC_PROGRESS}
        onShowMailbox={() => navigate(Routes.Mailbox)}
        onShowSettings={() => navigate(Routes.Settings)}
      />
      <Outlet />
    </>
  )
}
