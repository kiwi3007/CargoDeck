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
import KofiOverlay from './components/KofiOverlay';

import TabContent from './components/TabContent';

/**
 * NavigationTracker updates the UIContext with the last active path of specific sections.
 */
function NavigationTracker() {
  const location = useLocation();
  const { setLastLibraryPath } = useUI();

  useEffect(() => {
    if (location.pathname === '/library' || location.pathname.startsWith('/game/')) {
      setLastLibraryPath(location.pathname);
    }
  }, [location, setLastLibraryPath]);

  return null;
}

function App() {
  return (
    <UIProvider>
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

            {/* Game details view (persistent) */}
            <TabContent paths={['/game/']}>
              <Routes>
                <Route path="/game/:id" element={<GameDetails />} />
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
    </UIProvider>
  );
}

export default App;
