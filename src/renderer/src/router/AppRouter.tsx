import { HashRouter, Routes as RouterRoutes, Route, useNavigate } from 'react-router-dom';
import { WindowFrame } from '../components/shared/WindowFrame';
import { MailboxView } from '../views/MailboxView';
import { SettingsView } from '../views/SettingsView';
import { Onboarding } from '../views/Onboarding';
import { useTheme } from '../hooks/useTheme';
import { AuthProvider, useAuth } from './AuthContext';
import { ProtectedRoute, PublicRoute } from './guards';
import { PrivateLayout } from './PrivateLayout';
import { Routes } from './routes';

const SYNC_PROGRESS = 4;

const OnboardingRoute = () => {
  const { signIn } = useAuth();
  const { resolved, setTheme } = useTheme('light');

  return (
    <Onboarding
      onSignIn={signIn}
      onToggleTheme={() => setTheme(resolved === 'dark' ? 'light' : 'dark')}
    />
  );
};

const SettingsRoute = () => {
  const navigate = useNavigate();
  const { theme, setTheme } = useTheme('light');

  return <SettingsView theme={theme} onThemeChange={setTheme} onBack={() => navigate(Routes.Mailbox)} />;
};

export const AppRouter = () => {
  return (
    <AuthProvider>
      <HashRouter>
        <WindowFrame>
          <RouterRoutes>
            <Route element={<PublicRoute />}>
              <Route path={Routes.Onboarding} element={<OnboardingRoute />} />
            </Route>

            <Route element={<ProtectedRoute />}>
              <Route element={<PrivateLayout />}>
                <Route path={Routes.Mailbox} element={<MailboxView progress={SYNC_PROGRESS} />} />
                <Route path={Routes.Settings} element={<SettingsRoute />} />
              </Route>
            </Route>
          </RouterRoutes>
        </WindowFrame>
      </HashRouter>
    </AuthProvider>
  );
};
