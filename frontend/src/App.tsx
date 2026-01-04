import React from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';

import Library from './pages/Library';
import Settings from './pages/Settings';
import GameDetails from './pages/GameDetails';
import About from './pages/About';
import User from './pages/User';
import Navigation from './components/Navigation';
import ScannerStatus from './components/ScannerStatus';

function App() {
  return (
    <Router>
      <div className="app">
        <ScannerStatus />
        <Navigation />
        <main className="main-content">
          <Routes>
            <Route path="/" element={<Library />} />
            <Route path="/library" element={<Library />} />
            <Route path="/user" element={<User />} />
            <Route path="/game/:id" element={<GameDetails />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/about" element={<About />} />
          </Routes>
        </main>
      </div>
    </Router>
  );
}

export default App;
