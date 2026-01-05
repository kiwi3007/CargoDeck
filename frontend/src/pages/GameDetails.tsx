import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import axios from 'axios';
import { t, getLanguage } from '../i18n/translations';
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
  const [notification, setNotification] = useState<{ message: string, type: 'success' | 'error' } | null>(null);

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

  const handleDownload = async (url: string) => {
    if (downloadingUrl) return;

    setDownloadingUrl(url);
    try {
      const response = await axios.post('/api/v3/downloadclient/add', { url });
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
    if (sortField !== field) return '↕️';
    return sortOrder === 'asc' ? '🔼' : '🔽';
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
          <h1>{game.title}</h1>
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

      <div className="torrent-search">
        <h2>{t('searchTorrents')}</h2>
        <p>{t('willSearchProwlarr')} <strong>{game.title}</strong></p>
        <button onClick={handleSearchTorrents} disabled={searching}>
          {searching ? t('searching') : t('searchTorrents')}
        </button>
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
                <div className="col-age sortable" onClick={() => handleSort('age')}>{t('age')} {getSortIcon('age')}</div>
                <div className="col-title sortable" onClick={() => handleSort('title')}>{t('title')} {getSortIcon('title')}</div>
                <div className="col-indexer sortable" onClick={() => handleSort('indexer')}>{t('indexer')} {getSortIcon('indexer')}</div>
                <div className="col-platform">{t('platform')}</div>
                <div className="col-size sortable" onClick={() => handleSort('size')}>{t('size')} {getSortIcon('size')}</div>
                <div className="col-peers sortable" onClick={() => handleSort('seeders')}>{t('peers')} {getSortIcon('seeders')}</div>
                <div className="col-quality">{t('quality')}</div>
                <div className="col-actions">{t('download')}</div>
              </div>

              {sortedResults.map((result, index) => {
                const analysis = analyzeTorrent(result.title);
                const platformStyle = PLATFORM_CONFIG[analysis.detectedPlatform]?.color || '#45475a';

                return (
                  <div key={index} className={`results-row ${analysis.confidence}`}>
                    <div className="col-protocol">
                      <span className={`protocol-badge ${(result.protocol || 'torrent').toLowerCase()}`}>
                        {(result.protocol || 'TORRENT').toUpperCase()}
                      </span>
                    </div>

                    <div className="col-age">
                      <span className="age" title={new Date(result.publishDate).toLocaleString()}>
                        {result.formattedAge || t('unknown')}
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
                      <span className="platform-tag" style={{ backgroundColor: platformStyle }}>
                        {analysis.detectedPlatform}
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
                            {result.seeders ?? 0}
                          </span>
                          <span className="separator">/</span>
                          <span className="leechers">
                            {result.leechers ?? 0}
                          </span>
                        </div>
                      )}
                    </div>

                    <div className="col-source">
                      <span className={`provider-badge ${(result.provider || '').toLowerCase()}`}>
                        {result.provider || 'Unknown'}
                      </span>
                    </div>

                    <div className="col-actions">
                      <div className="download-buttons">
                        {result.magnetUrl && (
                          <button
                            className={`download-btn magnet ${downloadingUrl === result.magnetUrl ? 'loading' : ''}`}
                            title="Send to qBittorrent"
                            onClick={() => handleDownload(result.magnetUrl)}
                            disabled={!!downloadingUrl}
                          >
                            {downloadingUrl === result.magnetUrl ? '⏳' : '🧲'}
                          </button>
                        )}
                        {result.downloadUrl && (
                          <button
                            className={`download-btn direct ${downloadingUrl === result.downloadUrl ? 'loading' : ''}`}
                            title="Send to qBittorrent"
                            onClick={() => handleDownload(result.downloadUrl)}
                            disabled={!!downloadingUrl}
                          >
                            {downloadingUrl === result.downloadUrl ? '⏳' : '⬇️'}
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

      <div className="back-link">
        <Link to="/library">{t('backToLibrary')}</Link>
      </div>
    </div>
  );
};

export default GameDetails;
