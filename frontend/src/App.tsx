import { BrowserRouter as Router, Routes, Route, useLocation } from 'react-router-dom';
import React, { useEffect } from 'react';

import Library from './pages/Library';
import Settings from './pages/Settings';
import GameDetails from './pages/GameDetails';
import About from './pages/About';
import User from './pages/User';
import Status from './pages/Status';
import Navigation from './components/Navigation';
import ScannerStatus from './components/ScannerStatus';
import { useUI, UIProvider } from './context/UIContext';
import { SearchCacheProvider } from './context/SearchCacheContext';
import KofiOverlay from './components/KofiOverlay';

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

function useSSE() {
  useEffect(() => {
    // Connect to Go backend SSE stream for real-time library updates
    let es: EventSource | null = null;
    let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      try {
        es = new EventSource('/api/v3/events');
        es.addEventListener('LIBRARY_UPDATED', () => {
          window.dispatchEvent(new Event('LIBRARY_UPDATED_EVENT'));
        });
        es.addEventListener('AGENT_PROGRESS', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('AGENT_PROGRESS_EVENT', { detail: e.data }));
        });
        es.addEventListener('AGENT_JOB_QUEUED', (e: MessageEvent) => {
          window.dispatchEvent(new CustomEvent('AGENT_PROGRESS_EVENT', { detail: e.data }));
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
  }, []);
}

function App() {
  useSSE();
  return (
    <UIProvider>
      <SearchCacheProvider>
        <Router>
          <NavigationTracker />
          <div className="app">
            <KofiOverlay />
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

              <TabContent paths={['/status']}>
                <Status />
              </TabContent>
              <TabContent paths={['/user']}>
                <User />
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
