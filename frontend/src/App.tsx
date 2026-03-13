import { BrowserRouter as Router, Routes, Route, useLocation } from 'react-router-dom';
import React, { useEffect, useState } from 'react';
import axios from 'axios';

// authFetch wraps fetch() and injects the Authorization header from axios defaults.
// Use this anywhere fetch() is needed (e.g. streaming responses) instead of plain fetch().
export function authFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const token = (axios.defaults.headers.common as Record<string, string>)['Authorization'];
  const headers = new Headers(init?.headers);
  if (token) headers.set('Authorization', token);
  return fetch(input, { ...init, headers });
}

import Library from './pages/Library';
import Settings from './pages/Settings';
import GameDetails from './pages/GameDetails';
import About from './pages/About';
import Status from './pages/Status';
import Devices from './pages/Devices';
import Navigation from './components/Navigation';
import ScannerStatus from './components/ScannerStatus';
import LoginModal from './components/LoginModal';
import { useUI, UIProvider } from './context/UIContext';
import { SearchCacheProvider } from './context/SearchCacheContext';

import TabContent from './components/TabContent';

/**
 * NavigationTracker updates the UIContext with the last active path of specific sections.
 */
function NavigationTracker() {
  const location = useLocation();
  const { setLastLibraryPath, setLastSettingsPath } = useUI();

  useEffect(() => {
    if (location.pathname === '/library' || location.pathname.startsWith('/game/')) {
      setLastLibraryPath(location.pathname);
    }
    if (location.pathname === '/settings') {
      const fullPath = location.pathname + location.hash;
      setLastSettingsPath(fullPath);
    }
  }, [location, setLastLibraryPath, setLastSettingsPath]);

  return null;
}

function useSSE(password: string) {
  useEffect(() => {
    // Connect to Go backend SSE stream for real-time library updates
    let es: EventSource | null = null;
    let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      try {
        const url = password
          ? `/api/v3/events?auth=${encodeURIComponent(password)}`
          : '/api/v3/events';
        es = new EventSource(url);
        es.addEventListener('LIBRARY_UPDATED', () => {
          window.dispatchEvent(new Event('LIBRARY_UPDATED_EVENT'));
        });
        es.addEventListener('AGENT_PROGRESS', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('AGENT_PROGRESS_EVENT', { detail: e.data }));
        });
        es.addEventListener('AGENT_JOB_QUEUED', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('AGENT_PROGRESS_EVENT', { detail: e.data }));
        });
        es.addEventListener('AGENTS_UPDATED', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('AGENTS_UPDATED_EVENT', { detail: e.data }));
        });
        es.addEventListener('DOWNLOAD_QUEUE_UPDATED', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('DOWNLOAD_QUEUE_UPDATED_EVENT', { detail: e.data }));
        });
        es.addEventListener('AGENT_LOG_DATA', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('AGENT_LOG_DATA_EVENT', { detail: e.data }));
        });
        es.addEventListener('AGENT_SCRIPT_DATA', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('AGENT_SCRIPT_DATA_EVENT', { detail: e.data }));
        });
        es.addEventListener('GAME_UPDATE_AVAILABLE', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('GAME_UPDATE_AVAILABLE_EVENT', { detail: e.data }));
        });
        es.addEventListener('SAVE_CONFLICT', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('SAVE_CONFLICT_EVENT', { detail: e.data }));
        });
        es.onerror = () => {
          es?.close();
          // Reconnect after 5 seconds on error
          reconnectTimeout = setTimeout(connect, 5000);
        };
      } catch {
        // SSE not supported or server not available — fall back to polling
      }
    };

    connect();

    return () => {
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
      es?.close();
    };
  }, [password]);
}

function App() {
  const [authChecked, setAuthChecked] = useState(false);
  const [authRequired, setAuthRequired] = useState(false);
  const [password, setPassword] = useState('');
  const [showLogin, setShowLogin] = useState(false);

  useEffect(() => {
    // Check if auth is required, then restore session or show login
    fetch('/api/v3/auth/required')
      .then(r => r.json())
      .then((data: { required: boolean }) => {
        if (data.required) {
          setAuthRequired(true);
          const saved = sessionStorage.getItem('playerrAuth') || '';
          if (saved) {
            applyPassword(saved);
            setAuthChecked(true);
          } else {
            setShowLogin(true);
            setAuthChecked(true);
          }
        } else {
          setAuthChecked(true);
        }
      })
      .catch(() => {
        // Server unreachable — just proceed without auth
        setAuthChecked(true);
      });
  }, []);

  // Set axios interceptor + handle 401.
  // Re-check auth on any 401 — authRequired may be stale if the password was
  // set while the app was already open (e.g. via Settings).
  useEffect(() => {
    const id = axios.interceptors.response.use(
      r => r,
      err => {
        if (err.response?.status === 401) {
          sessionStorage.removeItem('playerrAuth');
          setPassword('');
          setAuthRequired(true);
          delete axios.defaults.headers.common['Authorization'];
          setShowLogin(true);
        }
        return Promise.reject(err);
      }
    );
    return () => axios.interceptors.response.eject(id);
  }, []);

  const applyPassword = (pw: string) => {
    setPassword(pw);
    if (pw) {
      axios.defaults.headers.common['Authorization'] = `Bearer ${pw}`;
    } else {
      delete axios.defaults.headers.common['Authorization'];
    }
  };

  const handleLoginSuccess = (pw: string) => {
    applyPassword(pw);
    setShowLogin(false);
    // Trigger all mounted pages to re-fetch now that the auth header is set.
    window.dispatchEvent(new Event('LIBRARY_UPDATED_EVENT'));
  };

  useSSE(password);

  if (!authChecked) return null;

  return (
    <UIProvider>
      <SearchCacheProvider>
        {showLogin && <LoginModal onSuccess={handleLoginSuccess} />}
        <Router>
          <NavigationTracker />
          <div className="app">
            <ScannerStatus />
            <Navigation />
            <main className="main-content">
              {/* Library list view */}
              <TabContent paths={['/', '/library']} className="no-padding">
                <Library />
              </TabContent>

              {/* Game details view (persistent once opened) */}
              <TabContent paths={['/game/']}>
                <Routes>
                  <Route path="/game/:id" element={<GameDetails />} />
                  {/* When NOT on a game route but the tab is still technically mounted (e.g. at /settings),
                    Routes will render nothing. However, since the TabContent is display:none, it's fine.
                    BUT if we want "Keep-Alive" for the SPECIFIC game we were viewing,
                    we'd need a way to keep the Route matching even if the URL changed.
                    React Router doesn't natively support this easily without a custom Switch.
                */}
                </Routes>
              </TabContent>

              <TabContent paths={['/devices']}>
                <Devices />
              </TabContent>
              <TabContent paths={['/status']}>
                <Status />
              </TabContent>
              <TabContent paths={['/settings']}>
                <Settings />
              </TabContent>
              <TabContent paths={['/about']}>
                <About />
              </TabContent>

              <Routes>
                {/* Other dynamic routes could go here */}
                <Route path="*" element={null} />
              </Routes>
            </main>
          </div>
        </Router>
      </SearchCacheProvider>
    </UIProvider>
  );
}

export default App;
