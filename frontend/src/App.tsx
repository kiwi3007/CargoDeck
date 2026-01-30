import React from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';

import Library from './pages/Library';
import Settings from './pages/Settings';
import GameDetails from './pages/GameDetails';
import About from './pages/About';
import User from './pages/User';
import Status from './pages/Status';
import Navigation from './components/Navigation';
import ScannerStatus from './components/ScannerStatus';
import { UIProvider } from './context/UIContext';
import KofiOverlay from './components/KofiOverlay';

import TabContent from './components/TabContent';

function App() {
  return (
    <UIProvider>
      <Router>
        <div className="app">
          <KofiOverlay />
          <ScannerStatus />
          <Navigation />
          <main className="main-content">
            <TabContent paths={['/', '/library']} className="no-padding">
              <Library />
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
              {/* Dynamic routes that should NOT be kept alive (e.g. GameDetails) */}
              <Route path="/game/:id" element={
                <div className="tab-container">
                  <GameDetails />
                </div>
              } />
              <Route path="*" element={null} />
            </Routes>
          </main>
        </div>
      </Router>
    </UIProvider>
  );
}

export default App;
