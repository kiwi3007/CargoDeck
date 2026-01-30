import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import axios from 'axios';
import GameCard from '../components/GameCard';
import ContextMenu, { ContextMenuOption } from '../components/ContextMenu';
import { t, getLanguage } from '../i18n/translations';
import appLogo from '../assets/app_logo.png';
import './Library.css';
import { useUI } from '../context/UIContext';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faTrash, faThLarge, faBars } from '@fortawesome/free-solid-svg-icons';
import { Virtuoso, VirtuosoGrid } from 'react-virtuoso';

interface Game {
  id: number;
  title: string;
  year: number;
  overview: string;
  images: {
    coverUrl?: string;
  };
  rating: number;
  genres: string[];
  platformId?: number;
  platform: { id?: number; name: string };
  status: string;
  steamId?: number;
  path?: string;
}

interface SearchResult {
  id: number;
  title: string;
  overview?: string;
  images: {
    coverUrl?: string;
  };
  year?: number;
  igdbId?: number;
  availablePlatforms?: string[];
}

interface Platform {
  id: number;
  name: string;
  slug: string;
}

const Library: React.FC = () => {
  const { toggleKofi } = useUI();
  const navigate = useNavigate();
  const [platforms, setPlatforms] = useState<Platform[]>([]);
  const [selectedPlatform, setSelectedPlatform] = useState('');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc');
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [showSearchResults, setShowSearchResults] = useState(false);
  const [games, setGames] = useState<Game[]>([]);
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [forceUpdateCounter, setForceUpdateCounter] = useState(0); // Force re-render trigger
  const [isIgdbConfigured, setIsIgdbConfigured] = useState(true); // Assume true until check
  const [showClearConfirm, setShowClearConfirm] = useState(false);

  const [contextMenu, setContextMenu] = useState<{ x: number, y: number, visible: boolean, game: Game | null }>({
    x: 0,
    y: 0,
    visible: false,
    game: null
  });

  const language = getLanguage();

  const handleContextMenu = (e: React.MouseEvent, game: Game) => {
    e.preventDefault();
    e.stopPropagation();
    console.log('Right click detected for game:', game.title);
    // alerta temporal para verificar que el clic derecho llega
    // window.alert('Clic derecho en: ' + game.title); 
    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      visible: true,
      game
    });
  };

  const handleDeleteGame = async (game: Game) => {
    try {
      await axios.delete(`/api/v3/game/${game.id}`);
      await loadGames();
    } catch (error: any) {
      console.error('Error deleting game:', error);
    }
  };

  // Cargar juegos de la biblioteca desde la API
  useEffect(() => {
    loadGames();
    loadPlatforms();
    checkIgdbConfig();

    // Listen for global library updates (e.g. from Auto-Scan in Settings)
    const handleLibraryUpdate = () => {
      console.log("[Library] Received update signal (EVENT). Loading games...");
      setForceUpdateCounter(prev => prev + 1); // FORCE React to re-render
      loadGames();
    };

    window.addEventListener('LIBRARY_UPDATED_EVENT', handleLibraryUpdate);
    return () => {
      window.removeEventListener('LIBRARY_UPDATED_EVENT', handleLibraryUpdate);
    };
  }, []);

  const loadGames = async () => {
    try {
      const response = await axios.get(`/api/v3/game?t=${Date.now()}`);
      setGames(response.data);
    } catch (error) {
      console.error('Error loading games:', error);
    }
  };

  const checkIgdbConfig = async () => {
    try {
      const response = await axios.get('/api/v3/settings/igdb');
      if (!response.data.clientId || !response.data.clientSecret) {
        setIsIgdbConfigured(false);
      } else {
        setIsIgdbConfigured(true);
      }
    } catch (error) {
      console.error('Error checking IGDB config:', error);
    }
  };

  const loadPlatforms = async () => {
    try {
      const response = await axios.get('/api/v3/platform');
      // Filter out Xbox and macOS platforms (but not SteamOS)
      let filteredPlatforms = response.data.filter((p: any) =>
        !p.name.includes('Xbox') && !p.name.match(/\b(mac|macos)\b/i)
      );

      // Group all PlayStation platforms into one
      const hasPlayStation = filteredPlatforms.some((p: any) =>
        p.name.toLowerCase().includes('playstation') || p.name.toLowerCase().includes('ps')
      );

      if (hasPlayStation) {
        // Remove individual PlayStation entries
        filteredPlatforms = filteredPlatforms.filter((p: any) =>
          !p.name.toLowerCase().includes('playstation') && !p.name.toLowerCase().match(/\bps[1-5]\b/i)
        );
        // Add unified PlayStation option
        filteredPlatforms.unshift({ id: 'playstation', name: 'PlayStation', slug: 'playstation' });
      }

      // Add SteamOS option manually for synced games
      filteredPlatforms.unshift({ id: 'steam', name: 'SteamOS', slug: 'steam' });

      setPlatforms(filteredPlatforms);
    } catch (error) {
      console.error('Error loading platforms:', error);
    }
  };

  const handleSearch = async () => {
    if (!searchQuery.trim()) return;

    setIsSearching(true);
    setShowSearchResults(true);
    try {
      const response = await axios.get(`/api/v3/game/lookup?term=${encodeURIComponent(searchQuery)}&lang=${language}`);
      setSearchResults(response.data);
    } catch (error) {
      console.error('Error searching games:', error);
      setSearchResults([]);
    } finally {
      setIsSearching(false);
    }
  };

  const handleClearLibrary = () => {
    setShowClearConfirm(true);
  };

  const confirmClearLibrary = async () => {
    try {
      await axios.delete('/api/v3/game/all');
      await loadGames();
    } catch (error) {
      console.error('Error clearing library:', error);
    } finally {
      setShowClearConfirm(false);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'Downloaded':
        return '#a6e3a1';
      case 'Downloading':
        return '#89b4fa';
      case 'Missing':
        return '#f38ba8';
      default:
        return '#313244';
    }
  };

  const translateStatus = (status: string) => {
    switch (status) {
      case 'Downloaded': return t('statusDownloaded');
      case 'Downloading': return t('statusDownloading');
      case 'Missing': return t('statusMissing');
      default: return t('statusUnknown');
    }
  };

  const handleAddGame = async (result: SearchResult) => {
    try {
      const platformId = selectedPlatform ? parseInt(selectedPlatform, 10) : 6;
      // Usar directamente el resultado de búsqueda como base para crear el juego
      const newGame: any = {
        title: result.title,
        year: result.year ?? 0,
        overview: result.overview ?? '',
        igdbId: result.igdbId ?? result.id,
        images: result.images,
        platformId,
        // GameStatus enum en backend: TBA=0, Announced=1, Released=2, Downloading=3, Downloaded=4, Missing=5
        status: 5,
        monitored: true
      };

      await axios.post('/api/v3/game', newGame);

      // Recargar la biblioteca
      await loadGames();

      // Cerrar resultados de búsqueda
      setShowSearchResults(false);
      setSearchQuery('');
      setSearchResults([]);
    } catch (error) {
      console.error('Error adding game:', error);
      alert(t('error'));
    }
  };

  const filteredGames = games.filter(game => {
    if (!selectedPlatform) return true;
    if (selectedPlatform === 'steam') return !!game.steamId;

    // Handle unified PlayStation filter
    if (selectedPlatform === 'playstation') {
      const platformName = game.platform?.name?.toLowerCase() || '';
      const path = game.path?.toLowerCase() || '';
      return platformName.includes('playstation') ||
        platformName.match(/\bps[1-5]\b/i) ||
        path.endsWith('.pkg') ||
        path.includes('/ps');
    }

    // Check for explicit platform ID match (loose equality for string/number)
    // Coerce everything to string for safe comparison
    const platformId = selectedPlatform.toString();
    if (game.platform?.id?.toString() === platformId || game.platformId?.toString() === platformId) {
      // SPECIAL CASUISTRY: If the platform is PC (ID 6 usually, but we check name if avail),
      // we might want to hide Steam games if the user intends "Non-Steam PC".
      // Let's rely on the user having a separate "Steam" filter.
      // If we are here, it matched the platform ID.
      // We check if the MATCHED platform is PC, and if so, exclude Steam games from it.
      const pData = platforms.find(p => p.id.toString() === platformId);
      if (pData && (pData.name.toLowerCase().includes('pc') || pData.name.toLowerCase().includes('windows'))) {
        if (game.steamId) return false;
      }
      return true;
    }

    // Fallback: Check file extensions for specific platforms if metadata is missing or partial
    const platformData = platforms.find(p => p.id.toString() === platformId);
    if (platformData) {
      const name = platformData.name.toLowerCase();
      const slug = platformData.slug?.toLowerCase() || '';
      const path = game.path?.toLowerCase() || '';

      // SPECIAL RULE: If filtering by PC, EXCLUDE Steam games (they have their own filter)
      if (name.includes('pc') || name.includes('windows') || slug.includes('pc')) {
        if (game.steamId) return false;
      }

      if (name.includes('switch') || slug.includes('switch') || name.includes('nintendo')) {
        return path.endsWith('.nsp') || path.endsWith('.xci') || path.endsWith('.nsz');
      }
      if (name.includes('ps4') || slug.includes('ps4')) {
        return path.endsWith('.pkg');
      }
    }

    return false;
  }).sort((a, b) => {
    const titleA = a.title?.toLowerCase() || '';
    const titleB = b.title?.toLowerCase() || '';
    if (sortOrder === 'asc') return titleA.localeCompare(titleB);
    return titleB.localeCompare(titleA);
  });

  return (
    <div className="library">
      <div className="library-header">
        <div className="header-left">
          <div className="library-stats">
            <span style={{ textTransform: 'uppercase', fontWeight: 'bold' }}>
              {filteredGames.length} {t('gamesCount')}
            </span>
          </div>
        </div>

        <div className="header-right">
          <div className="search-bar-mini">
            <input
              type="text"
              placeholder={t('searchPlaceholder')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyPress={(e) => e.key === 'Enter' && handleSearch()}
              size={Math.max((t('searchPlaceholder') || '').length, searchQuery.length) + 2}
            />
            <button
              onClick={handleSearch}
              disabled={isSearching || !searchQuery.trim()}
              className="search-btn-mini"
            >
              {t('search')}
            </button>
          </div>
        </div>
      </div>

      <div className="library-controls-bar">
        <div className="control-group">
          <span className="control-label">{t('platform')}:</span>
          <select
            value={selectedPlatform}
            onChange={(e) => setSelectedPlatform(e.target.value)}
            className="platform-filter-select"
          >
            <option value="">{t('allPlatforms')}</option>
            {platforms.map(platform => (
              <option key={platform.id} value={platform.id.toString()}>
                {platform.name}
              </option>
            ))}
          </select>
        </div>

        <div className="control-group right">
          <button
            className="control-btn sort-btn"
            onClick={() => setSortOrder(prev => prev === 'asc' ? 'desc' : 'asc')}
            title={`Sort: ${sortOrder === 'asc' ? 'A-Z' : 'Z-A'}`}
          >
            {sortOrder === 'asc' ? 'A-Z' : 'Z-A'}
          </button>

          <button
            className="control-btn clear-btn"
            onClick={handleClearLibrary}
            title={t('clearLibrary')}
          >
            <FontAwesomeIcon icon={faTrash} />
          </button>

          <div className="view-toggle">
            <button
              className={`view-btn ${viewMode === 'grid' ? 'active' : ''}`}
              onClick={() => setViewMode('grid')}
              title={t('grid')}
            >
              <FontAwesomeIcon icon={faThLarge} />
            </button>
            <button
              className={`view-btn ${viewMode === 'list' ? 'active' : ''}`}
              onClick={() => setViewMode('list')}
              title={t('list')}
            >
              <FontAwesomeIcon icon={faBars} />
            </button>
          </div>
        </div>
      </div>

      {showSearchResults && (
        <div className="search-results-overlay">
          <div className="search-results-modal">
            <div className="search-results-header">
              <h3>{t('searchResults')}</h3>
              <button className="close-btn" onClick={() => setShowSearchResults(false)}>×</button>
            </div>
            <div className="search-results-list">
              {isSearching ? (
                <div className="search-loading">
                  <p>{t('searching')}</p>
                </div>
              ) : searchResults.length === 0 ? (
                <div className="no-results">
                  <p>{t('noGamesFound')} "{searchQuery}"</p>
                </div>
              ) : (
                searchResults.map(result => (
                  <div key={result.id} className="search-result-item">
                    {result.images?.coverUrl && (
                      <img src={result.images.coverUrl} alt={result.title} className="result-cover" />
                    )}
                    <div className="result-info">
                      <h4>{result.title}</h4>
                      <div className="result-meta-row" style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', marginBottom: '0.5rem' }}>
                        {typeof result.year === 'number' && result.year > 0 && (
                          <span className="result-year">
                            {result.year}
                          </span>
                        )}
                        {result.availablePlatforms && result.availablePlatforms.map(p => (
                          <span key={p} className="platform-badge" style={{
                            fontSize: '0.7rem',
                            padding: '2px 6px',
                            borderRadius: '4px',
                            backgroundColor: '#45475a',
                            color: '#cdd6f4'
                          }}>
                            {p}
                          </span>
                        ))}
                      </div>
                      {result.overview && (
                        <p className="result-summary">{result.overview.substring(0, 150)}...</p>
                      )}
                    </div>
                    <button
                      className="add-game-btn"
                      onClick={() => handleAddGame(result)}
                    >
                      {t('addToLibrary')}
                    </button>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      )
      }


      {
        filteredGames.length === 0 ? (
          <div className="empty-library">
            <div className="empty-icon" onClick={toggleKofi} style={{ cursor: 'pointer' }}>
              <img src={appLogo} alt="Playerr" className="empty-lib-logo" />
            </div>
            <h3>
              {searchQuery
                ? t('noGamesFound')
                : (!isIgdbConfigured ? t('configureIgdbToStart') : t('noGamesInLibrary'))
              }
            </h3>
            <p>
              {searchQuery ? '' : (isIgdbConfigured ? t('useSearchBar') : '')}
            </p>
          </div>
        ) : viewMode === 'grid' ? (
          <VirtuosoGrid
            style={{ height: 'calc(100vh - 180px)', width: '100%', padding: '2rem' }}
            totalCount={filteredGames.length}
            listClassName="game-grid"
            itemContent={(index) => {
              const game = filteredGames[index];
              return (
                <GameCard
                  key={game.id}
                  game={game}
                  onClick={() => {
                    console.log('Navigating to game details', game.id);
                    navigate(`/game/${game.id}`);
                  }}
                  onContextMenu={(e) => handleContextMenu(e, game)}
                  onDelete={() => handleDeleteGame(game)}
                />
              );
            }}
          />
        ) : (
          <Virtuoso
            style={{ height: 'calc(100vh - 180px)', width: '100%', padding: '2rem' }}
            totalCount={filteredGames.length}
            itemContent={(index) => {
              const game = filteredGames[index];
              return (
                <div
                  key={game.id}
                  className="game-list-item"
                  onClick={() => navigate(`/game/${game.id}`)}
                  onContextMenu={(e) => handleContextMenu(e, game)}
                  style={{ marginBottom: '0.75rem' }}
                >
                  {game.images?.coverUrl ? (
                    <img src={game.images.coverUrl} alt={game.title} className="list-cover" />
                  ) : (
                    <div className="list-cover-placeholder">?</div>
                  )}
                  <div className="list-info">
                    <h3>{game.title || 'Untitled'}</h3>
                    <div className="list-meta">
                      <span>{game.year || 'N/A'}</span>
                      {game.platform?.name && <span>{game.platform.name}</span>}
                      <span
                        className="list-status-badge"
                        style={{
                          backgroundColor: getStatusColor(game.status),
                          color: ['Missing', 'Downloading', 'Downloaded'].includes(game.status) ? '#11111b' : '#cdd6f4'
                        }}
                      >
                        {translateStatus(game.status)}
                      </span>
                    </div>
                  </div>
                  <div className="list-rating">
                    {typeof game.rating === 'number' && game.rating > 0 ? (
                      <span>{Math.round(game.rating)}%</span>
                    ) : (
                      <span className="no-rating">N/A</span>
                    )}
                    <button
                      className="list-delete-btn"
                      title={t('deleteFromLibrary')}
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteGame(game);
                      }}
                    >
                      ×
                    </button>
                  </div>
                </div>
              );
            }}
          />
        )
      }

      <ContextMenu
        x={contextMenu.x}
        y={contextMenu.y}
        visible={contextMenu.visible}
        options={[
          {
            label: t('deleteFromLibrary'),
            icon: <FontAwesomeIcon icon={faTrash} />,
            danger: true,
            onClick: () => contextMenu.game && handleDeleteGame(contextMenu.game)
          }
        ]}
        onClose={() => setContextMenu({ ...contextMenu, visible: false })}
      />

      {showClearConfirm && (
        <div className="modal-overlay">
          <div className="modal">
            <div className="modal-header">
              <h3>{t('clearLibrary')}</h3>
              <button className="modal-close" onClick={() => setShowClearConfirm(false)}>×</button>
            </div>
            <div className="modal-content">
              <p>{t('clearLibraryConfirm')}</p>
              <div className="modal-actions">
                <button
                  className="btn-secondary"
                  onClick={() => setShowClearConfirm(false)}
                >
                  {t('cancel')}
                </button>
                <button
                  className="btn-danger"
                  onClick={confirmClearLibrary}
                >
                  {t('delete')}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div >
  );
};

export default Library;
