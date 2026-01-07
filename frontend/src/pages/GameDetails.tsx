import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import axios from 'axios';
import { t, getLanguage } from '../i18n/translations';
import GameCorrectionModal from '../components/GameCorrectionModal';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faPen, faFolderOpen, faDownload, faGamepad, faMagnet, faSpinner, faSort, faSortUp, faSortDown, faArrowUp, faArrowDown } from '@fortawesome/free-solid-svg-icons';
import './GameDetails.css';

interface Game {
  id: number;
  title: string;
  year: number;
  overview?: string;
  storyline?: string;
  images: {
    coverUrl?: string;
    backgroundUrl?: string;
    screenshots?: string[];
  };
  rating?: number;
  genres: string[];
  platform?: {
    name: string;
  };
  status: string | number;
}

interface TorrentResult {
  title: string;
  guid: string;
  downloadUrl: string;
  magnetUrl: string;
  infoUrl: string;
  indexerId: number;
  indexerName?: string;
  indexer?: string; // Matches backend JSON
  indexerFlags: string[];
  size: number;
  seeders?: number;
  leechers?: number;
  totalPeers?: number;
  publishDate: string;
  age: number;
  ageHours: number;
  ageMinutes: number;
  category: string;
  protocol: string;
  languages: string[];
  quality: string;
  releaseGroup: string;
  source: string;
  container: string;
  codec: string;
  resolution: string;
  // Added formatted properties from backend
  formattedSize: string;
  formattedAge: string;
  // Alternative date fields for robustness
  publishedAt?: string;
  pubDate?: string;
  provider: string; // Added provider field
}

const GameDetails: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const [game, setGame] = useState<Game | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searching, setSearching] = useState(false);
  const [results, setResults] = useState<TorrentResult[]>([]);
  const [sortField, setSortField] = useState<keyof TorrentResult | null>('seeders');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const [downloadingUrl, setDownloadingUrl] = useState<string | null>(null);
  const [notification, setNotification] = useState<{ message: string, type: 'success' | 'error' | 'info' } | null>(null);
  const [showCorrectionModal, setShowCorrectionModal] = useState(false);
  const [activeTab, setActiveTab] = useState<'search' | 'files' | 'none'>('search'); // 'search' by default to keep existing behavior? Or none? User said "Search Game" is one function. Let's make it toggleable.
  // Actually, standard behavior was "Search Torrents" always visible at bottom. 
  // User wants a MENU. 
  // Let's make "Search" show/hide the search section.

  useEffect(() => {
    if (notification) {
      const timer = setTimeout(() => {
        setNotification(null);
      }, 3000);
      return () => clearTimeout(timer);
    }
  }, [notification]);

  const language = getLanguage();

  useEffect(() => {
    const loadGame = async () => {
      if (!id) return;
      try {
        const response = await axios.get(`/api/v3/game/${id}?lang=${language}`);
        setGame(response.data);
      } catch (err: any) {
        setError(err.response?.data?.message || t('error'));
      } finally {
        setLoading(false);
      }
    };

    loadGame();
  }, [id, language]);


  const handleSort = (field: keyof TorrentResult) => {
    if (sortField === field) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortField(field);
      setSortOrder('desc');
    }
  };

  const handleDownload = async (url: string, protocol?: string) => {
    if (downloadingUrl) return;

    setDownloadingUrl(url);
    try {
      const response = await axios.post('/api/v3/downloadclient/add', {
        url,
        protocol: protocol
      });
      setNotification({ message: response.data.message || t('downloadStarted'), type: 'success' });
    } catch (error: any) {
      console.error('Download failed:', error);
      const errorMessage = error.response?.data?.message || t('failedToDownload');
      setNotification({ message: errorMessage, type: 'error' });
    } finally {
      setDownloadingUrl(null);
    }
  };

  const sortedResults = [...results].sort((a, b) => {
    if (!sortField) return 0;

    let aValue = a[sortField];
    let bValue = b[sortField];

    // Handle nulls
    if (aValue === undefined || aValue === null) return 1;
    if (bValue === undefined || bValue === null) return -1;

    if (typeof aValue === 'string' && typeof bValue === 'string') {
      return sortOrder === 'asc'
        ? aValue.localeCompare(bValue)
        : bValue.localeCompare(aValue);
    }

    if (typeof aValue === 'number' && typeof bValue === 'number') {
      return sortOrder === 'asc' ? aValue - bValue : bValue - aValue;
    }

    return 0;
  });

  const getSortIcon = (field: keyof TorrentResult) => {
    if (sortField !== field) return <FontAwesomeIcon icon={faSort} style={{ opacity: 0.3, marginLeft: '5px' }} />;
    return sortOrder === 'asc' ? <FontAwesomeIcon icon={faSortUp} style={{ marginLeft: '5px' }} /> : <FontAwesomeIcon icon={faSortDown} style={{ marginLeft: '5px' }} />;
  };

  const getSeedersClass = (seeders?: number) => {
    if (!seeders || seeders === 0) return 'danger';
    if (seeders > 50) return 'excellent';
    if (seeders > 10) return 'good';
    return 'warning';
  };

  const PLATFORM_CONFIG: Record<string, { categories: number[], keywords: string[], negativeKeywords: string[], extensions: string[], color: string }> = {
    'PC': {
      categories: [4000, 4010, 4050],
      keywords: ['PC', 'WINDOWS', 'WIN64', 'WIN32', '.EXE', 'WINE', 'GOG-GAMES', 'STEAM', 'CRACK', 'REPACK', 'FITGIRL', 'DODI', 'ELAMIGOS'],
      negativeKeywords: ['PS3', 'PS4', 'PS5', 'SWITCH', 'XBOX', 'NSW'],
      extensions: ['.exe', '.iso', '.bin'],
      color: '#4CAF50'
    },
    'Nintendo Switch': {
      categories: [1000, 1030],
      keywords: ['SWITCH', 'NSW', 'NSP', 'XCI', 'NSZ'],
      negativeKeywords: ['PS4', 'PC', 'XBOX', 'WII'],
      extensions: ['.nsp', '.xci', '.nsz'],
      color: '#e60012'
    },
    'PlayStation 4': {
      categories: [1000, 1080],
      keywords: ['PS4', 'PLAYSTATION 4', 'CUSA', 'PKG'],
      negativeKeywords: ['PS5', 'PC', 'SWITCH'],
      extensions: ['.pkg'],
      color: '#003087'
    },
    'PlayStation 5': {
      categories: [1000],
      keywords: ['PS5', 'PLAYSTATION 5', 'PPSA'],
      negativeKeywords: ['PS4', 'PC', 'SWITCH'],
      extensions: [],
      color: '#003087'
    },
    'Xbox One': {
      categories: [1000],
      keywords: ['XBOX ONE', 'XB1'],
      negativeKeywords: ['PS4', 'PC', 'SWITCH'],
      extensions: [],
      color: '#107c10'
    },
    'Xbox Series': {
      categories: [1000],
      keywords: ['XBOX SERIES', 'XBSX', 'XSX'],
      negativeKeywords: ['PS4', 'PC', 'SWITCH'],
      extensions: [],
      color: '#107c10'
    }
  };

  type PlatformType = 'PC' | 'PlayStation' | 'Xbox' | 'Nintendo' | 'Unknown';

  const GetPlatformInfo = (categoryId: number): { name: string, icon: string, type: PlatformType } => {
    switch (categoryId) {
      // ==========================================
      // 🖥️ PC & MAC
      // ==========================================
      case 4000: // PC General
      case 4010: // PC 0day
      case 4020: // PC ISO
      case 4040: // PC Mobile
      case 4050: // PC Games (Standard)
      case 14050: // PC Games (Extended)
      case 100400: // TPB PC General
      case 100401: // TPB PC
      case 104050: // User specific extended
        return { name: "PC", icon: "mdi-microsoft-windows", type: 'PC' };

      case 4030: // Mac
      case 100402: // TPB Mac
        return { name: "Mac", icon: "mdi-apple", type: 'PC' };

      // ==========================================
      // 🔵 SONY PLAYSTATION
      // ==========================================
      case 1080: // PS3
      case 101080: // PS3 Extended
      case 100403: // TPB PSx (A veces mezcla)
        return { name: "PS3", icon: "mdi-sony-playstation", type: 'PlayStation' };

      case 1180: // PS4 (Standard Newznab)
      case 101100: // PS4 (Extended)
        return { name: "PS4", icon: "mdi-sony-playstation", type: 'PlayStation' };

      case 1020: // PSP
      case 101020:
        return { name: "PSP", icon: "mdi-sony-playstation", type: 'PlayStation' };

      case 1120: // PS Vita
      case 101120:
        return { name: "PS Vita", icon: "mdi-sony-playstation", type: 'PlayStation' };

      // ==========================================
      // 🟢 MICROSOFT XBOX
      // ==========================================
      case 1040: // Xbox Original
      case 101040:
        return { name: "Xbox", icon: "mdi-microsoft-xbox", type: 'Xbox' };

      case 1050: // Xbox 360
      case 101050:
      case 1070: // 360 DLC
      case 100404: // TPB Xbox360
        return { name: "Xbox 360", icon: "mdi-microsoft-xbox", type: 'Xbox' };

      case 1140: // Xbox One
      case 101090: // Xbox One Extended
        return { name: "Xbox One", icon: "mdi-microsoft-xbox", type: 'Xbox' };

      // ==========================================
      // 🔴 NINTENDO
      // ==========================================
      case 101035: // Switch (El ID más común ahora)
      case 101110: // Switch Alternativo
      case 101111: // Switch Update/DLC
        return { name: "Switch", icon: "mdi-nintendo-switch", type: 'Nintendo' };

      case 1030: // Wii
      case 101030:
      case 100405: // TPB Wii
        return { name: "Wii", icon: "mdi-nintendo-wii", type: 'Nintendo' };

      case 1130: // Wii U
      case 101130:
        return { name: "Wii U", icon: "mdi-nintendo-wiiu", type: 'Nintendo' };

      case 1010: // NDS
      case 101010:
        return { name: "DS", icon: "mdi-nintendo-game-boy", type: 'Nintendo' };

      case 1110: // 3DS
        return { name: "3DS", icon: "mdi-nintendo-3ds", type: 'Nintendo' };

      // ==========================================
      // 📦 OTROS / GENÉRICOS
      // ==========================================
      case 1000: // Console General
        return { name: "Console", icon: "mdi-gamepad-variant", type: 'Unknown' };

      default:
        // Si es un 1xxx desconocido, es consola
        if (categoryId >= 1000 && categoryId < 2000) return { name: "Console", icon: "mdi-gamepad-variant", type: 'Unknown' };
        // Si es un 4xxx desconocido, es PC
        if (categoryId >= 4000 && categoryId < 5000) return { name: "PC", icon: "mdi-laptop", type: 'PC' };

        return { name: "Unknown", icon: "mdi-help-circle", type: 'Unknown' };
    }
  };

  const SCENE_GROUPS = ['FLT', 'CODEX', 'RUNE', 'TENOKE', 'SKIDROW', 'RELOADED', 'PROPHET', 'CPY', 'EMPRESS', 'RAZOR1911', 'GOLDBERG'];
  const REPACK_GROUPS = ['FITGIRL', 'DODI', 'ELAMIGOS', 'KAOS', 'XATAB'];

  const analyzeTorrent = (title: string) => {
    const t = title.toUpperCase();
    let detectedPlatform = 'Game';
    let confidence: 'match' | 'mismatch' | 'unknown' = 'unknown';
    const tags: string[] = [];

    // Detect Platform
    for (const [platformName, config] of Object.entries(PLATFORM_CONFIG)) {
      const hasKeyword = config.keywords.some(k => t.includes(k));
      const hasNegative = config.negativeKeywords.some(k => t.includes(k));

      if (hasKeyword && !hasNegative) {
        detectedPlatform = platformName;
        break;
      }
    }

    // Special case for generic PC keywords if not found
    if (detectedPlatform === 'Game') {
      if (t.includes('LINUX') || t.includes('WINE')) detectedPlatform = 'Linux';
    }

    // Determine Confidence relative to current game
    if (game?.platform) {
      if (detectedPlatform === game.platform.name ||
        (game.platform.name.includes('PC') && detectedPlatform === 'PC') ||
        (game.platform.name.includes('Switch') && detectedPlatform === 'Nintendo Switch')) {
        confidence = 'match';
      } else if (detectedPlatform !== 'Game' && detectedPlatform !== 'Linux') {
        confidence = 'mismatch';
      }
    }

    // Extract Extra Tags
    if (SCENE_GROUPS.some(g => t.includes(g))) tags.push('Scene');
    if (REPACK_GROUPS.some(g => t.includes(g))) tags.push('Repack');
    if (t.includes('FIX')) tags.push('Fix');
    if (t.includes('UPDATE')) tags.push('Update');
    if (t.includes('GOG')) tags.push('GOG');
    if (t.includes('STEAM')) tags.push('Steam');

    return { detectedPlatform, confidence, tags };
  };

  const handleSearchTorrents = async () => {
    if (!game) return;
    setSearching(true);
    setResults([]);
    setError(null);

    // Get categories based on platform
    let cats = '';
    if (game.platform) {
      const config = PLATFORM_CONFIG[game.platform.name] ||
        Object.values(PLATFORM_CONFIG).find(c => game.platform!.name.includes('PC'));
      if (config) {
        cats = config.categories.join(',');
      }
    }

    try {
      const response = await axios.get('/api/v3/search', {
        params: { query: game.title, categories: cats }
      });
      setResults(response.data);
    } catch (err: any) {
      setError(err.response?.data?.error || t('error'));
    } finally {
      setSearching(false);
    }
  };

  const handleCorrectionSave = async (updates: any) => {
    if (!game) return;
    try {
      await axios.put(`/api/v3/game/${game.id}`, updates);
      setNotification({ message: t('gameUpdated' as any) || 'Juego actualizado', type: 'success' });
      setShowCorrectionModal(false);
      // Reload game to reflect changes
      const response = await axios.get(`/api/v3/game/${game.id}?lang=${language}`);
      setGame(response.data);
    } catch (err: any) {
      console.error(err);
      setNotification({ message: t('errorUpdating' as any) || 'Error al actualizar', type: 'error' });
    }
  };
  const handleInstall = async () => {
    try {
      setNotification({ message: t('searchingInstaller' as any) || 'Buscando instalador...', type: 'info' });
      const res = await axios.post(`/api/v3/game/${id}/install`);
      setNotification({ message: `${t('installerLaunched' as any) || 'Instalador lanzado'}: ${res.data.path}`, type: 'success' });
    } catch (err: any) {
      console.error(err);
      setNotification({ message: err.response?.data || t('errorLaunchingInstaller' as any) || 'Error al lanzar instalador', type: 'error' });
    }
  };

  if (loading) {
    return <div className="game-details"><p>{t('loadingGame')}</p></div>;
  }

  if (error || !game) {
    return (
      <div className="game-details">
        <p>{error || t('gameNotFound')}</p>
        <Link to="/library">{t('backToLibrary')}</Link>
      </div>
    );
  }

  return (
    <div className="game-details">
      <div className="game-details-header">
        <div className="game-details-poster">
          {game.images.coverUrl ? (
            <img src={game.images.coverUrl} alt={game.title} />
          ) : (
            <div className="placeholder">?</div>
          )}
        </div>
        <div className="game-details-info">
          <div className="title-row" style={{ display: 'flex', alignItems: 'center', gap: '15px' }}>
            <h1>{game.title}</h1>
          </div>

          <div className="game-actions-menu">
            <button
              className={`action-btn ${activeTab === 'search' ? 'active' : ''}`}
              onClick={() => {
                setActiveTab('search');
                handleSearchTorrents();
              }}
              title={t('searchLinks' as any) || 'Buscar Enlaces'}
            >
              <FontAwesomeIcon icon={faSearch} />
              <span>{t('search' as any) || 'Buscar'}</span>
            </button>
            <button
              className="action-btn"
              onClick={() => setShowCorrectionModal(true)}
              title={t('correctMetadata' as any) || 'Corregir Metadatos'}
            >
              <FontAwesomeIcon icon={faPen} />
              <span>{t('correct' as any) || 'Corregir'}</span>
            </button>

            <button
              className="action-btn"
              onClick={handleInstall}
              title={t('install' as any) || 'Instalar'}
            >
              <FontAwesomeIcon icon={faDownload} />
              <span>{t('install' as any) || 'Instalar'}</span>
            </button>
            <button
              className="action-btn"
              onClick={() => console.log('Play clicked')}
              title={t('play' as any) || 'Jugar'}
            >
              <FontAwesomeIcon icon={faGamepad} />
              <span>{t('play' as any) || 'Jugar'}</span>
            </button>
          </div>
          <div className="meta">
            <span>{game.year}</span>
            {game.platform && <span>{game.platform.name}</span>}
            {game.rating && <span>{Math.round(game.rating)}%</span>}
          </div>
          {game.genres && game.genres.length > 0 && (
            <div className="genres">
              {game.genres.join(' / ')}
            </div>
          )}
          {game.overview && (
            <p className="overview">{game.overview}</p>
          )}
        </div>
      </div>

      {activeTab === 'search' && (results.length > 0 || error || searching) && (
        <div className="torrent-search">



          {searching && (
            <div className="search-loading">
              <FontAwesomeIcon icon={faSearch} spin />
              <p>{t('searching') || 'Buscando...'}</p>
            </div>
          )}

          {error && <p className="error">{error}</p>}


          {results.length > 0 && (
            <div className="results-container">
              {notification && (
                <div className={`download-notification ${notification.type}`}>
                  {notification.message}
                </div>
              )}
              <div className="results-header">
                <h4>{t('searchResults')} ({results.length} {t('resultsFound')})</h4>
              </div>

              <div className="results-table">
                <div className="results-header-row">
                  <div className="col-protocol sortable" onClick={() => handleSort('protocol')}>{t('protocol')} {getSortIcon('protocol')}</div>
                  <div className="col-title sortable" onClick={() => handleSort('title')}>{t('title')} {getSortIcon('title')}</div>
                  <div className="col-indexer sortable" onClick={() => handleSort('indexer')}>{t('indexer')} {getSortIcon('indexer')}</div>
                  <div className="col-platform">{t('platform')}</div>
                  <div className="col-size sortable" onClick={() => handleSort('size')}>{t('size')} {getSortIcon('size')}</div>
                  <div className="col-peers sortable" onClick={() => handleSort('seeders')}>{t('peers')} {getSortIcon('seeders')}</div>
                  <div className="col-actions">{t('download')}</div>
                </div>

                {sortedResults.map((result, index) => {
                  const analysis = analyzeTorrent(result.title);
                  const platformStyle = PLATFORM_CONFIG[analysis.detectedPlatform]?.color || '#45475a';

                  // Try to resolve explicit category name
                  let explicitPlatform = '';
                  let explicitPlatformType: PlatformType | null = null;

                  if (result.category) {
                    const catIds = result.category.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));
                    // Prioritize finding a detailed match (skipping general ones if detailed exists)
                    // But our GetPlatformInfo returns generic names for 1000/4000 too.
                    // Let's iterate and find the "best" one. 

                    // Sort IDs? Or just take first valid? Usually only 1 category per item.
                    // Sometimes multiple: 1000, 1010. We want 1010.

                    // Let's assume the highest ID often carries the most specific info in Newznab (e.g. 1010 > 1000)
                    // Or use the one that returns a name != "Console" and != "PC" and != "Unknown"

                    for (const cid of catIds) {
                      const info = GetPlatformInfo(cid);
                      if (info.name !== "Console" && info.name !== "Unknown") {
                        explicitPlatform = info.name;
                        explicitPlatformType = info.type;
                        break; // Found a specific one
                      }

                      // Only set generic if we haven't found anything yet AND it's not Unknown
                      if (!explicitPlatform && info.name !== "Unknown") {
                        explicitPlatform = info.name;
                        explicitPlatformType = info.type;
                      }
                    }
                  }

                  // Final display platform: Explicit Category > Detected Title Analysis
                  // If explicit is empty (because all were Unknown), it falls back to detected.
                  const displayPlatform = explicitPlatform || analysis.detectedPlatform;

                  // Adjust color using Type
                  let finalColor = platformStyle;
                  if (explicitPlatformType) {
                    switch (explicitPlatformType) {
                      case 'Nintendo': finalColor = PLATFORM_CONFIG['Nintendo Switch'].color; break;
                      case 'PlayStation': finalColor = PLATFORM_CONFIG['PlayStation 5'].color; break;
                      case 'Xbox': finalColor = PLATFORM_CONFIG['Xbox Series'].color; break;
                      case 'PC': finalColor = PLATFORM_CONFIG['PC'].color; break;
                      default: finalColor = '#45475a'; break;
                    }
                  }

                  return (
                    <div key={index} className={`results-row ${analysis.confidence}`}>
                      <div className="col-protocol">
                        <span className={`protocol-badge ${(result.protocol || 'torrent').toLowerCase()}`}>
                          {(result.protocol || 'TORRENT').toUpperCase()}
                        </span>
                      </div>



                      <div className="col-title">
                        <div className="title-content">
                          {result.infoUrl ? (
                            <a href={result.infoUrl} target="_blank" rel="noopener noreferrer" className="title-link">
                              {result.title}
                            </a>
                          ) : (
                            <span className="title-text">{result.title}</span>
                          )}
                          <div className="title-meta">
                            {result.releaseGroup && (
                              <span className="release-group">{result.releaseGroup}</span>
                            )}
                            {analysis.tags.map((tag, i) => (
                              <span key={i} className={`title-tag ${tag.toLowerCase()}`}>[{tag}]</span>
                            ))}
                          </div>
                        </div>
                      </div>

                      <div className="col-indexer">
                        <span className="indexer-name">{result.indexer || result.indexerName}</span>
                      </div>

                      <div className="col-platform">
                        <span className="platform-tag" style={{ backgroundColor: finalColor }} title={`Category IDs: ${result.category || 'None'}`}>
                          {displayPlatform}
                        </span>
                      </div>

                      <div className="col-size">
                        <span className="size">{result.formattedSize || `${(result.size / (1024 * 1024 * 1024)).toFixed(2)} GB`}</span>
                      </div>

                      <div className="col-peers">
                        {result.protocol?.toLowerCase() === 'usenet' || result.protocol?.toLowerCase() === 'nzb' ? (
                          <span className="peers-info">-</span>
                        ) : (
                          <div className="peers-info">
                            <span className={`seeders ${getSeedersClass(result.seeders)}`}>
                              <FontAwesomeIcon icon={faArrowUp} /> {result.seeders ?? 0}
                            </span>
                            <span className="separator">/</span>
                            <span className="leechers">
                              <FontAwesomeIcon icon={faArrowDown} /> {result.leechers ?? 0}
                            </span>
                          </div>
                        )}
                      </div>



                      <div className="col-actions">
                        <div className="download-buttons">
                          {result.magnetUrl && (
                            <button
                              className={`download-btn magnet ${downloadingUrl === result.magnetUrl ? 'loading' : ''}`}
                              title="Send to Download Client"
                              onClick={() => handleDownload(result.magnetUrl, result.protocol)}
                              disabled={!!downloadingUrl}
                            >
                              {downloadingUrl === result.magnetUrl ? <FontAwesomeIcon icon={faSpinner} spin /> : <FontAwesomeIcon icon={faMagnet} />}
                            </button>
                          )}
                          {result.downloadUrl && (
                            <button
                              className={`download-btn direct ${downloadingUrl === result.downloadUrl ? 'loading' : ''}`}
                              title="Send to Download Client"
                              onClick={() => handleDownload(result.downloadUrl, result.protocol)}
                              disabled={!!downloadingUrl}
                            >
                              {downloadingUrl === result.downloadUrl ? <FontAwesomeIcon icon={faSpinner} spin /> : <FontAwesomeIcon icon={faDownload} />}
                            </button>
                          )}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}

      <div className="back-link">
        <Link to="/library">{t('backToLibrary')}</Link>
      </div>

      {showCorrectionModal && game && (
        <GameCorrectionModal
          game={game}
          language={language}
          onClose={() => setShowCorrectionModal(false)}
          onSave={handleCorrectionSave}
        />
      )}
    </div>
  );
};

export default GameDetails;
