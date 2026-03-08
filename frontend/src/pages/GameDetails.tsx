import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import axios from 'axios';
import { t, getLanguage, useTranslation } from '../i18n/translations';
import { useSearchCache } from '../context/SearchCacheContext';
import GameCorrectionModal from '../components/GameCorrectionModal';
import UninstallModal from '../components/UninstallModal';
import SwitchInstallerModal from '../components/SwitchInstallerModal';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faPen, faFolderOpen, faDownload, faGamepad, faMagnet, faSpinner, faSort, faSortUp, faSortDown, faArrowUp, faArrowDown, faTrash, faMicrochip, faDatabase } from '@fortawesome/free-solid-svg-icons';
import './GameDetails.css';
import VersionSelectorModal, { VersionOption } from '../components/VersionSelectorModal';

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
  isInstallable?: boolean;
  availablePlatforms?: string[];
  steamId?: number;
  path?: string;
  installPath?: string;
  sizeOnDisk?: number;
  uninstallerPath?: string;
  downloadPath?: string;
  executablePath?: string;
  canPlay?: boolean;
  gameFiles?: GameFile[];
  currentVersion?: string;
  latestVersion?: string;
  updateAvailable?: boolean;
}

interface GameFile {
  id: number;
  relativePath: string;
  releaseGroup?: string;
  quality?: string;
  size: number;
}

interface SaveSnapshot {
  id: string;      // "{agentId}/{timestamp}"
  agentId: string;
  timestamp: string;
  sizeBytes: number;
}

interface ServerFile {
  relativePath: string;
  name: string;
  size: number;
  gameFileId?: number;
  quality?: string;
  releaseGroup?: string;
}

interface SavePathsInfo {
  source: 'custom' | 'manifest' | 'fallback' | 'none';
  paths: string[];
  savePath?: string; // custom path if source=custom
  steamId?: number;
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

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + '\u00a0' + sizes[i];
}

const GameDetails: React.FC = () => {
  useTranslation(); // Subscribe to language changes
  const { id } = useParams<{ id: string }>();
  const { getCacheForGame, setCacheForGame } = useSearchCache();
  const [game, setGame] = useState<Game | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searching, setSearching] = useState(false);
  // Initialize results from cache if available (for initial render)
  const [results, setResults] = useState<TorrentResult[]>(() => {
    if (id) {
      const cached = getCacheForGame(parseInt(id));
      return cached || [];
    }
    return [];
  });

  // Update results when id changes (in case component is reused)
  useEffect(() => {
    if (id) {
      const cached = getCacheForGame(parseInt(id));
      if (cached) {
        setResults(cached);
      } else {
        setResults([]);
      }
    }
  }, [id, getCacheForGame]);
  const [sortField, setSortField] = useState<keyof TorrentResult | null>('publishDate');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');
  const [downloadingUrl, setDownloadingUrl] = useState<string | null>(null);
  const [notification, setNotification] = useState<{ message: string, type: 'success' | 'error' | 'info' } | null>(null);
  const [showCorrectionModal, setShowCorrectionModal] = useState(false);
  const [showUninstallModal, setShowUninstallModal] = useState(false);
  const [showInstallWarning, setShowInstallWarning] = useState(false);
  const [showSwitchModal, setShowSwitchModal] = useState(false);
  const [showVersionModal, setShowVersionModal] = useState(false);
  const [versionOptions, setVersionOptions] = useState<VersionOption[]>([]);
  const [actionType, setActionType] = useState<'install' | 'play' | null>(null);
  const [activeTab, setActiveTab] = useState<'search' | 'files' | 'saves' | 'none'>('search');
  const [serverFiles, setServerFiles] = useState<ServerFile[]>([]);
  const [serverFilesLoading, setServerFilesLoading] = useState(false);
  const [deletingFile, setDeletingFile] = useState<Record<string, boolean>>({});
  const [saveSnapshots, setSaveSnapshots] = useState<SaveSnapshot[]>([]);
  const [savesLoading, setSavesLoading] = useState(false);
  const [savePathsInfo, setSavePathsInfo] = useState<SavePathsInfo | null>(null);
  const [customPathInput, setCustomPathInput] = useState('');
  const [savingCustomPath, setSavingCustomPath] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [updateBannerDismissed, setUpdateBannerDismissed] = useState(false);

  // ---- Remote agent install ----
  interface InstallPath { path: string; label: string; freeBytes: number; }
  interface ActiveJob { jobId: string; gameTitle: string; status: string; message: string; percent: number; }
  interface InstalledGameOnAgent { title: string; installPath: string; exePath?: string; exeCandidates?: string[]; scriptPath?: string; sizeBytes: number; hasShortcut: boolean; version?: string; }
  interface AgentInfo { id: string; name: string; platform: string; status: string; installPaths?: InstallPath[]; currentJob?: ActiveJob; installedGames?: InstalledGameOnAgent[]; }
  // Agent scan reports directory names as titles; safeName replaces path-unsafe chars with '-'.
  // Normalize both sides so "Story of Seasons: Grand Bazaar" matches "Story of Seasons- Grand Bazaar".
  const agentTitleMatches = (agentTitle: string, libraryTitle: string) => {
    const norm = (s: string) => s.replace(/[/\\:*?"<>|]/g, '-');
    return agentTitle === libraryTitle || agentTitle === norm(libraryTitle);
  };
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [showAgentDropdown, setShowAgentDropdown] = useState(false);
  const [agentJobProgress, setAgentJobProgress] = useState<{ status: string; message: string; percent: number } | null>(null);
  const [deletingFromAgent, setDeletingFromAgent] = useState<Record<string, boolean>>({});
  const [exeDropdownOpen, setExeDropdownOpen] = useState<Record<string, boolean>>({});
  const [restoringOnAgent, setRestoringOnAgent] = useState<Record<string, boolean>>({});
  const [requestingLog, setRequestingLog] = useState<Record<string, boolean>>({});
  const [logModal, setLogModal] = useState<{ agentName: string; gameTitle: string; content: string } | null>(null);
  const pendingLogRequest = React.useRef<string | null>(null);

  // Exe picker modal state
  const [exeCandidates, setExeCandidates] = useState<{ name: string; relPath: string; type: 'installer' | 'game'; fromArchive?: string }[]>([]);
  const [showExePicker, setShowExePicker] = useState(false);
  const [selectedExeName, setSelectedExeName] = useState('');
  const [pendingInstall, setPendingInstall] = useState<{ agentId: string; installDir?: string } | null>(null);

  useEffect(() => {
    const applyAgentList = (list: AgentInfo[]) => {
      setAgents(list);
      const withJob = list.find(a => a.currentJob);
      if (withJob?.currentJob) {
        const j = withJob.currentJob;
        setAgentJobProgress({ status: j.status, message: j.message, percent: j.percent });
      }
    };
    // Initial fetch before first SSE push
    axios.get('/api/v3/agent').then(r => applyAgentList(r.data || [])).catch(() => {});
    const handler = (e: Event) => {
      try { applyAgentList(JSON.parse((e as CustomEvent).detail) || []); } catch { /* ignore */ }
    };
    window.addEventListener('AGENTS_UPDATED_EVENT', handler);
    return () => window.removeEventListener('AGENTS_UPDATED_EVENT', handler);
  }, []);

  useEffect(() => {
    const handler = (e: Event) => {
      try {
        const prog = JSON.parse((e as CustomEvent).detail);
        setAgentJobProgress({ status: prog.status, message: prog.message, percent: prog.percent });
      } catch { /* ignore */ }
    };
    window.addEventListener('AGENT_PROGRESS_EVENT', handler);
    return () => window.removeEventListener('AGENT_PROGRESS_EVENT', handler);
  }, []);

  useEffect(() => {
    const handler = (e: Event) => {
      try {
        const payload = JSON.parse((e as CustomEvent).detail);
        if (payload.requestId && payload.requestId === pendingLogRequest.current) {
          const agentName = agents.find(a => a.installedGames?.some(g => agentTitleMatches(g.title, payload.gameTitle)))?.name ?? 'Agent';
          setLogModal({ agentName, gameTitle: payload.gameTitle, content: payload.content });
          setRequestingLog(prev => {
            const next = { ...prev };
            delete next[payload.gameTitle];
            return next;
          });
          pendingLogRequest.current = null;
        }
      } catch { /* ignore */ }
    };
    window.addEventListener('AGENT_LOG_DATA_EVENT', handler);
    return () => window.removeEventListener('AGENT_LOG_DATA_EVENT', handler);
  }, [agents]);

  const formatFreeSpace = (bytes: number): string => {
    if (bytes < 0) return '';
    const gb = bytes / (1024 ** 3);
    if (gb >= 1) return gb.toFixed(0) + '\u00a0GB';
    return (bytes / (1024 ** 2)).toFixed(0) + '\u00a0MB';
  };

  const dispatchInstall = async (agentId: string, installDir?: string, selectedExe?: string) => {
    try {
      await axios.post(`/api/v3/agent/${agentId}/install`, {
        gameId: parseInt(id!),
        installDir,
        ...(selectedExe ? { selectedExe } : {}),
      });
      setNotification({ message: 'Install job sent to agent', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.message || 'Failed to dispatch install job', type: 'error' });
    }
  };

  const handleRemoteInstall = async (agentId: string, installDir?: string) => {
    setShowAgentDropdown(false);
    if (!id) return;
    if (!game?.path && (!game?.gameFiles || game.gameFiles.length === 0) && !game?.downloadPath) {
      setNotification({ message: t('noGameFilesFound'), type: 'error' });
      return;
    }
    try {
      const r = await axios.get<{ candidates: { name: string; relPath: string; type: 'installer' | 'game'; fromArchive?: string }[] }>(
        `/api/v3/agent/${agentId}/install-preview?gameId=${id}`
      );
      const cands = r.data.candidates ?? [];
      if (cands.length > 1) {
        setExeCandidates(cands);
        setPendingInstall({ agentId, installDir });
        setSelectedExeName(cands.find(c => c.type === 'installer')?.name ?? cands[0].name);
        setShowExePicker(true);
        return;
      }
      await dispatchInstall(agentId, installDir, cands[0]?.name);
    } catch {
      await dispatchInstall(agentId, installDir);
    }
  };

  const handleViewLog = async (agentId: string, gameTitle: string) => {
    setRequestingLog(prev => ({ ...prev, [gameTitle]: true }));
    try {
      const r = await axios.post<{ requestId: string }>(`/api/v3/agent/${agentId}/readlog`, { gameTitle });
      pendingLogRequest.current = r.data.requestId;
    } catch (err: any) {
      alert(`Failed to request log: ${err.response?.data?.error || err.message}`);
      setRequestingLog(prev => { const next = { ...prev }; delete next[gameTitle]; return next; });
    }
  };

  const handleChangeExe = async (agentId: string, title: string, exePath: string) => {
    try {
      await axios.post(`/api/v3/agent/${agentId}/change-exe`, { title, exePath });
      setNotification({ message: 'Exe change sent to agent', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Failed to change exe', type: 'error' });
    }
  };

  const handleDeleteFromAgent = async (agentId: string, ig: InstalledGameOnAgent) => {
    const agentName = agents.find(a => a.id === agentId)?.name || 'device';
    if (!window.confirm(`Delete "${ig.title}" from ${agentName}?${ig.hasShortcut ? '\nSteam shortcut will also be removed.' : ''}`)) return;
    const key = `${agentId}:${ig.installPath}`;
    setDeletingFromAgent(prev => ({ ...prev, [key]: true }));
    try {
      await axios.delete(`/api/v3/agent/${agentId}/game`, {
        data: { title: ig.title, installPath: ig.installPath, removeShortcut: ig.hasShortcut }
      });
      setNotification({ message: `Deleted from ${agentName}`, type: 'success' });
    } catch (err: any) {
      setNotification({ message: `Delete failed: ${err.response?.data?.error || err.message}`, type: 'error' });
    } finally {
      setDeletingFromAgent(prev => ({ ...prev, [key]: false }));
    }
  };

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
        setSearchTerm(prev => prev || response.data.title);
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

  const loadSaveSnapshots = async () => {
    if (!game) return;
    setSavesLoading(true);
    try {
      const r = await axios.get<SaveSnapshot[]>(`/api/v3/save/${game.id}`);
      setSaveSnapshots(r.data || []);
    } catch {
      setSaveSnapshots([]);
    } finally {
      setSavesLoading(false);
    }
  };

  const loadSavePathsInfo = async () => {
    if (!game) return;
    try {
      const r = await axios.get<SavePathsInfo>(`/api/v3/save/paths-info?gameId=${game.id}`);
      setSavePathsInfo(r.data);
      setCustomPathInput(r.data.savePath ?? '');
    } catch {
      setSavePathsInfo(null);
    }
  };

  const loadServerFiles = async () => {
    if (!game) return;
    setServerFilesLoading(true);
    try {
      const r = await axios.get<ServerFile[]>(`/api/v3/game/${game.id}/server-files`);
      setServerFiles(r.data || []);
    } catch {
      setServerFiles([]);
    } finally {
      setServerFilesLoading(false);
    }
  };

  const handleDeleteServerFile = async (file: ServerFile) => {
    if (!game) return;
    if (!window.confirm(`Delete "${file.name}" from the server?`)) return;
    setDeletingFile(prev => ({ ...prev, [file.relativePath]: true }));
    try {
      await axios.delete(`/api/v3/game/${game.id}/server-file`, { data: { relativePath: file.relativePath } });
      setServerFiles(prev => prev.filter(f => f.relativePath !== file.relativePath));
    } catch (err: any) {
      alert('Delete failed: ' + (err.response?.data?.error || err.message));
    } finally {
      setDeletingFile(prev => { const n = { ...prev }; delete n[file.relativePath]; return n; });
    }
  };

  const handleSetCustomPath = async () => {
    if (!game) return;
    setSavingCustomPath(true);
    try {
      await axios.patch(`/api/v3/save/${game.id}/path`, { savePath: customPathInput.trim() });
      await loadSavePathsInfo();
      setNotification({ message: customPathInput.trim() ? 'Custom save path set.' : 'Custom save path cleared.', type: 'success' });
    } catch (err: any) {
      setNotification({ message: 'Failed to set path: ' + (err.response?.data?.error || err.message), type: 'error' });
    } finally {
      setSavingCustomPath(false);
    }
  };

  const handleDeleteSnapshot = async (snapshotId: string) => {
    if (!game) return;
    if (!window.confirm('Delete this save snapshot?')) return;
    try {
      await axios.delete(`/api/v3/save/${game.id}/${snapshotId}`);
      setSaveSnapshots(prev => prev.filter(s => s.id !== snapshotId));
    } catch (err: any) {
      alert(`Delete failed: ${err.response?.data?.error || err.message}`);
    }
  };

  const handleRestoreOnAgent = async (agentId: string) => {
    if (!game) return;
    if (!window.confirm(`Restore the latest save for "${game.title}" on this device?\n\nThis will overwrite current save files.`)) return;
    setRestoringOnAgent(prev => ({ ...prev, [agentId]: true }));
    try {
      await axios.post(`/api/v3/agent/${agentId}/restore-save`, { gameId: game.id, title: game.title });
      setNotification({ message: 'Restore requested — the agent will apply the latest save.', type: 'success' });
    } catch (err: any) {
      setNotification({ message: 'Restore failed: ' + (err.response?.data?.error || err.message), type: 'error' });
    } finally {
      setRestoringOnAgent(prev => { const n = { ...prev }; delete n[agentId]; return n; });
    }
  };

  const handleSearchTorrents = async (overrideTerm?: string) => {
    if (!game) return;
    setSearching(true);
    setResults([]);
    setError(null);
    setHasSearched(false);

    const query = overrideTerm ?? (searchTerm || game.title);

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
        params: { query, categories: cats }
      });
      setResults(response.data);
      if (id) {
        setCacheForGame(parseInt(id), response.data);
      }
    } catch (err: any) {
      setError(err.response?.data?.error || t('error'));
    } finally {
      setSearching(false);
      setHasSearched(true);
    }
  };

  const handleCorrectionSave = async (updates: any) => {
    if (!game) return;
    try {
      await axios.put(`/api/v3/game/${game.id}`, updates);
      setNotification({ message: t('gameUpdated'), type: 'success' });
      setShowCorrectionModal(false);
      // Reload game to reflect changes
      const response = await axios.get(`/api/v3/game/${game.id}?lang=${language}`);
      setGame(response.data);
    } catch (err: any) {
      console.error(err);
      setNotification({ message: t('errorUpdating'), type: 'error' });
    }
  };
  const handleInstallClick = () => {
    if (!game?.path && (!game?.gameFiles || game.gameFiles.length === 0) && !game?.downloadPath) {
      setNotification({ message: t('noGameFilesFound'), type: 'error' });
      return;
    }
    setActionType('install');
    setShowInstallWarning(true);
  };

  const getAvailableVersions = (type: 'install' | 'play'): VersionOption[] => {
    const options: VersionOption[] = [];
    if (!game) return [];

    // 1. Primary Version
    if (game.path) {
      const primaryTag = game.status === 'InstallerDetected' || (game.isInstallable && !game.canPlay) ? 'Installer' : 'Playable';
      // Backend status is vague, check our computed logic or trust the tag logic above
      // Better: check if we are filtering.

      // Let's assume standard logic:
      let tag = 'Playable';
      if (game.status === 'InstallerDetected' || (game.isInstallable && !game.canPlay)) tag = 'Installer';

      // Override: If checking specifically for 'play', and this is 'Installer', don't add? 
      // Actually, let's add all and filter later or handle in filtering.
      options.push({
        label: game.title + (tag === 'Installer' ? ' (Installer)' : ''),
        path: game.executablePath || game.path,
        details: game.executablePath || game.path,
        tag: tag
      });
    }

    // 2. Download path (incoming, not yet installed)
    if (!game.path && game.downloadPath) {
      options.push({
        label: `${game.title} (Downloaded)`,
        path: game.downloadPath,
        details: game.downloadPath,
        tag: 'Installer'
      });
    }

    // 3. Alternate Versions
    if (game.gameFiles && game.gameFiles.length > 0) {
      game.gameFiles.forEach(f => {
        options.push({
          label: f.releaseGroup || 'Alternate Version',
          path: f.relativePath,
          details: f.relativePath,
          tag: f.quality || 'Playable'
        });
      });
    }

    // Filter based on action
    if (type === 'install') {
      return options.filter(o => o.tag === 'Installer');
    } else {
      return options.filter(o => o.tag === 'Playable');
    }
  };

  const onWarningConfirmed = () => {
    setShowInstallWarning(false);

    // Fallback: If no strict installers found (e.g. status is weird), show all?
    // User wants strict behavior.
    let options = getAvailableVersions('install');

    // Safety fallback: if 0 options found but we clicked Install, maybe the logic tagged it wrong. 
    // Just show everything if filtering yields 0?
    if (options.length === 0) options = getAvailableVersions('install').length === 0 ? [] : options; // wait logic loop
    // Let's rely on getAvailableVersions first. If empty, maybe fall back to ALL options just in case?
    if (options.length === 0) {
      // If empty, use unfiltered to be safe (maybe tag is missing)
      const all = [];
      if (game?.path) all.push({ label: game.title, path: game.path, details: game.path, tag: 'Unknown' });
      if (game?.gameFiles) game.gameFiles.forEach(f => all.push({ label: f.releaseGroup || 'Alt', path: f.relativePath, details: f.relativePath, tag: f.quality || 'Unknown' }));
      options = all;
    }

    if (options.length > 1) {
      setVersionOptions(options);
      setShowVersionModal(true);
    } else {
      // If only 1 option, just launch it? 
      // Yes.
      executeInstall(options.length > 0 ? options[0].path : undefined);
    }
  };

  const executeInstall = async (overridePath?: string) => {
    setShowVersionModal(false);

    // If event object came through (from onClick), ignore it
    const actualPath = typeof overridePath === 'string' ? overridePath : undefined;

    try {
      setNotification({ message: t('searchingInstaller'), type: 'info' });
      let url = `/api/v3/game/${id}/install`;
      if (actualPath && actualPath.length > 0) {
        url += `?path=${encodeURIComponent(actualPath)}`;
      }

      const res = await axios.post(url);
      setNotification({ message: `${t('installerLaunched')}: ${res.data.path}`, type: 'success' });
    } catch (err: any) {
      console.error(err);
      setNotification({ message: err.response?.data || t('errorLaunchingInstaller'), type: 'error' });
    }
  };

  const handlePlay = async () => {
    setActionType('play');
    console.log('[handlePlay] Checking versions...');

    const options = getAvailableVersions('play');

    if (options.length > 1) {
      setVersionOptions(options);
      setShowVersionModal(true);
      return;
    }

    // If 1 or 0 (default), execute directly
    await executePlay(options.length === 1 ? options[0].path : undefined);
  };

  const executePlay = async (path?: string) => {
    // alert(`Debug: Launching game ${id} (Steam ID: ${game?.steamId})`);
    console.log('[executePlay] Launching. Path:', path);
    try {
      setNotification({ message: t('launchingGame'), type: 'info' });
      let url = `/api/v3/game/${id}/play`;
      if (path) url += `?path=${encodeURIComponent(path)}`;

      await axios.post(url);
      setNotification({ message: t('gameLaunched'), type: 'success' });
    } catch (err: any) {
      console.error(err);
      setNotification({ message: err.response?.data?.error || t('errorLaunchingGame'), type: 'error' });
    }
  };

  const handleRunUninstaller = async () => {
    try {
      setNotification({ message: t('launchingUninstaller') || 'Launching Uninstaller...', type: 'info' });
      await axios.post(`/api/v3/game/${id}/uninstall`);
      setNotification({ message: t('uninstallerLaunched') || 'Uninstaller Launched', type: 'success' });
    } catch (err: any) {
      console.error(err);
      setNotification({ message: err.response?.data || t('errorLaunchingUninstaller') || 'Error launching uninstaller', type: 'error' });
    }
  };

  const handleDeleteGame = async (deleteLibraryFiles: boolean, deleteDownloadFiles: boolean, targetLibraryPath?: string, targetDownloadPath?: string) => {
    if (!id) return;
    try {
      setNotification({ message: t('deletingGame') || 'Deleting...', type: 'info' });
      let url = `/api/v3/game/${id}?deleteFiles=${deleteLibraryFiles}&deleteDownloadFiles=${deleteDownloadFiles}`;
      if (targetLibraryPath) url += `&targetPath=${encodeURIComponent(targetLibraryPath)}`;
      if (targetDownloadPath) url += `&downloadPath=${encodeURIComponent(targetDownloadPath)}`;

      await axios.delete(url);
      setNotification({ message: t('gameDeleted') || 'Game Deleted', type: 'success' });
      // Redirect to library after short delay
      setTimeout(() => {
        window.location.href = '/library';
      }, 1000);
    } catch (err: any) {
      console.error(err);
      setNotification({ message: t('errorDeletingGame') || 'Error deleting game', type: 'error' });
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

  const isSwitchGame = (() => {
    if (!game) return false;
    const pathLower = game.path?.toLowerCase() || '';
    const isSwitchFile = pathLower.endsWith('.nsp') || pathLower.endsWith('.xci') || pathLower.endsWith('.nsz') || pathLower.endsWith('.xcz');
    const isSwitchPlatform = game.platform?.name?.toLowerCase().includes('switch') || false;
    return isSwitchFile || isSwitchPlatform;
  })();

  return (
    <div className="game-details">
      <div className="breadcrumb-nav" style={{ marginBottom: '15px' }}>
        <Link to="/library" style={{
          color: '#89b4fa',
          textDecoration: 'none',
          fontSize: '0.9rem',
          display: 'inline-flex',
          alignItems: 'center',
          gap: '5px'
        }}>
          ← {t('library')}
        </Link>
      </div>
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
              title={t('searchLinks')}
            >
              <FontAwesomeIcon icon={faSearch} />
              <span>{t('search')}</span>
            </button>
            <button
              className={`action-btn ${activeTab === 'saves' ? 'active' : ''}`}
              onClick={() => { setActiveTab('saves'); loadSaveSnapshots(); loadSavePathsInfo(); }}
              title="Save backups"
            >
              <FontAwesomeIcon icon={faFolderOpen} />
              <span>Saves</span>
            </button>
            <button
              className={`action-btn ${activeTab === 'files' ? 'active' : ''}`}
              onClick={() => { setActiveTab('files'); loadServerFiles(); }}
              title="Files & Devices"
            >
              <FontAwesomeIcon icon={faDatabase} />
              <span>Files</span>
            </button>
            <button
              className="action-btn"
              onClick={() => setShowCorrectionModal(true)}
              title={t('correctMetadata')}
            >
              <FontAwesomeIcon icon={faPen} />
              <span>{t('correct')}</span>
            </button>

            <button
              className="action-btn"
              onClick={() => setShowUninstallModal(true)}
              title={t('uninstallTitle')}
            >
              <FontAwesomeIcon icon={faTrash} />
              <span>{t('remove')}</span>
            </button>

            {(!isSwitchGame) && (() => {
              const onlineAgents = agents.filter(a => a.status === 'online');
              if (onlineAgents.length > 0) {
                return (
                  <div style={{ position: 'relative', display: 'inline-block' }}>
                    <button
                      className={`action-btn ${game.isInstallable && !game.canPlay ? 'install-ready' : ''}`}
                      onClick={() => setShowAgentDropdown(v => !v)}
                      title="Install to remote device"
                    >
                      <FontAwesomeIcon icon={faDownload} />
                      <span>{t('install')} ▾</span>
                    </button>
                    {showAgentDropdown && (
                      <div style={{
                        position: 'absolute', top: '100%', left: 0, zIndex: 100,
                        background: '#1e1e2e', border: '1px solid #45475a', borderRadius: '6px',
                        minWidth: '200px', padding: '4px 0'
                      }}>
                        {onlineAgents.map(a => {
                          const paths = a.installPaths || [];
                          if (paths.length > 1) {
                            return (
                              <React.Fragment key={a.id}>
                                <div style={{ padding: '7px 14px 3px', color: '#a6adc8', fontSize: '0.78rem', fontWeight: 600, cursor: 'default', userSelect: 'none' }}>
                                  📱 {a.name}
                                </div>
                                {paths.map((p, i) => (
                                  <button key={i}
                                    style={{ display: 'flex', justifyContent: 'space-between', width: '100%', textAlign: 'left', padding: '6px 14px 6px 24px', background: 'none', border: 'none', color: '#cdd6f4', cursor: 'pointer', fontSize: '0.85rem' }}
                                    onClick={() => handleRemoteInstall(a.id, p.path)}
                                  >
                                    <span><span style={{ opacity: 0.4, marginRight: '4px' }}>›</span>{p.label}</span>
                                    {p.freeBytes >= 0 && <span style={{ opacity: 0.45, fontSize: '0.75rem', marginLeft: '8px' }}>{formatFreeSpace(p.freeBytes)}</span>}
                                  </button>
                                ))}
                              </React.Fragment>
                            );
                          }
                          return (
                            <button key={a.id}
                              style={{ display: 'block', width: '100%', textAlign: 'left', padding: '8px 14px', background: 'none', border: 'none', color: '#cdd6f4', cursor: 'pointer', fontSize: '0.9rem' }}
                              onClick={() => handleRemoteInstall(a.id, paths[0]?.path)}
                            >
                              📱 {a.name}
                            </button>
                          );
                        })}
                        <div style={{ borderTop: '1px solid #45475a', margin: '4px 0' }} />
                        <button
                          style={{ display: 'block', width: '100%', textAlign: 'left', padding: '8px 14px', background: 'none', border: 'none', color: '#cdd6f4', cursor: 'pointer', fontSize: '0.9rem' }}
                          onClick={handleInstallClick}
                        >
                          🖥️ This server
                        </button>
                        <a
                          href={`/api/v3/game/${id}/install-script`}
                          download
                          style={{ display: 'block', padding: '8px 14px', color: '#89b4fa', fontSize: '0.9rem', textDecoration: 'none' }}
                        >
                          📄 Download script
                        </a>
                      </div>
                    )}
                  </div>
                );
              }
              return (
                <React.Fragment>
                  <button
                    className={`action-btn ${game.isInstallable && !game.canPlay ? 'install-ready' : ''}`}
                    onClick={handleInstallClick}
                    title={t('install')}
                  >
                    <FontAwesomeIcon icon={faDownload} />
                    <span>{t('install')}</span>
                  </button>
                  <a
                    href={`/api/v3/game/${id}/install-script`}
                    download
                    className="action-btn"
                    title="Download install script"
                  >
                    <span>📄</span>
                    <span>Script</span>
                  </a>
                </React.Fragment>
              );
            })()}

            {isSwitchGame && (
              <button
                className="action-btn switch-usb"
                onClick={() => setShowSwitchModal(true)}
                title="Install to Switch via USB"
                style={{ background: '#e60012', color: 'white' }}
              >
                <FontAwesomeIcon icon={faMicrochip} />
                <span>USB Install</span>
              </button>
            )}

            {(!isSwitchGame) && (
              <button
                className={`action-btn ${game.canPlay ? 'play-ready' : ''}`}
                onClick={handlePlay}
                title={t('play')}
              >
                <FontAwesomeIcon icon={faGamepad} />
                <span>{t('play')}</span>
              </button>
            )}
          </div>

          {agentJobProgress && (
            <div style={{ marginTop: '10px', padding: '10px 14px', background: 'rgba(137,180,250,0.1)', borderRadius: '8px', border: '1px solid rgba(137,180,250,0.3)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.85rem', color: '#89b4fa', marginBottom: '6px' }}>
                <span>{agentJobProgress.message}</span>
                <span>{agentJobProgress.percent}%</span>
              </div>
              <div style={{ background: 'rgba(255,255,255,0.1)', borderRadius: '4px', height: '6px', overflow: 'hidden' }}>
                <div style={{
                  width: `${agentJobProgress.percent}%`,
                  height: '100%',
                  background: agentJobProgress.status === 'done' ? '#a6e3a1' : agentJobProgress.status === 'failed' ? '#f38ba8' : '#89b4fa',
                  transition: 'width 0.3s ease'
                }} />
              </div>
            </div>
          )}

          <div className="meta">
            <span>{game.year}</span>
            {game.platform && <span>{game.platform.name}</span>}
            {game.rating && <span>{Math.round(game.rating)}%</span>}
          </div>

          {game.availablePlatforms && game.availablePlatforms.length > 0 && (
            <div className="platforms-list" style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginTop: '8px', marginBottom: '8px' }}>
              {game.availablePlatforms.map(p => (
                <span key={p} style={{
                  backgroundColor: 'rgba(255, 255, 255, 0.1)',
                  padding: '2px 8px',
                  borderRadius: '4px',
                  fontSize: '0.8rem',
                  color: '#cdd6f4',
                  border: '1px solid rgba(255, 255, 255, 0.05)'
                }}>
                  {p}
                </span>
              ))}
            </div>
          )}

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

      {game.updateAvailable && !updateBannerDismissed && (
        <div className="update-banner">
          <div className="update-banner-info">
            <span className="update-banner-icon">⬆</span>
            <span>
              <strong>Update Available</strong>
              {game.currentVersion && <span className="update-banner-version"> Installed: v{game.currentVersion}</span>}
              {game.latestVersion && <span className="update-banner-version"> → Found: v{game.latestVersion}</span>}
            </span>
          </div>
          <div className="update-banner-actions">
            <button
              className="update-banner-btn primary"
              onClick={() => {
                setSearchTerm(game.title);
                setActiveTab('search');
                handleSearchTorrents(game.title);
              }}
            >
              Search Now
            </button>
            <button
              className="update-banner-btn"
              onClick={() => setUpdateBannerDismissed(true)}
            >
              Dismiss
            </button>
          </div>
        </div>
      )}

      {
        game && (
          <UninstallModal
            isOpen={showUninstallModal}
            onClose={() => setShowUninstallModal(false)}
            onRunUninstaller={handleRunUninstaller}
            onDelete={handleDeleteGame}
            gameTitle={game.title}
            gamePath={game.path}
            uninstallerPath={game.uninstallerPath}
            downloadPath={game.downloadPath}
          />
        )
      }

      {
        game && game.path && (
          <SwitchInstallerModal
            isOpen={showSwitchModal}
            onClose={() => setShowSwitchModal(false)}
            filePath={game.path}
            fileName={game.title} // Or get filename from path
          />
        )
      }

      <div className="back-link" style={{ marginTop: '20px', marginBottom: '10px' }}>
        <Link to="/library">{t('backToLibrary')}</Link>
      </div>

      {
        activeTab === 'search' && (results.length > 0 || error || searching || hasSearched) && (
          <div className="torrent-search">

            <div className="search-term-row" style={{ display: 'flex', gap: '8px', marginBottom: '12px' }}>
              <input
                type="text"
                value={searchTerm}
                onChange={e => setSearchTerm(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') handleSearchTorrents(); }}
                style={{ flex: 1, padding: '6px 10px', background: '#313244', border: '1px solid #45475a', borderRadius: '4px', color: '#cdd6f4', fontSize: '14px' }}
              />
              <button
                onClick={() => handleSearchTorrents()}
                disabled={searching}
                style={{ padding: '6px 14px', background: '#89b4fa', color: '#1e1e2e', border: 'none', borderRadius: '4px', cursor: 'pointer', fontWeight: 600 }}
              >
                <FontAwesomeIcon icon={faSearch} />
              </button>
            </div>

            {searching && (
              <div className="search-loading">
                <FontAwesomeIcon icon={faSearch} spin />
                <p>{t('searching') || 'Searching...'}</p>
              </div>
            )}

            {hasSearched && !searching && results.length === 0 && !error && (
              <p style={{ color: '#a6adc8', textAlign: 'center', padding: '20px' }}>
                No results found. Check your indexer settings (Prowlarr/Jackett) or try a different search term.
              </p>
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
                    <div className="col-date sortable" onClick={() => handleSort('publishDate')}>Date {getSortIcon('publishDate')}</div>
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



                        <div className="col-date">
                          {result.publishDate ? (() => {
                            const d = new Date(result.publishDate);
                            return isNaN(d.getTime()) ? '-' : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
                          })() : '-'}
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
        )
      }



      {
        showCorrectionModal && game && (
          <GameCorrectionModal
            game={game}
            language={language}
            onClose={() => setShowCorrectionModal(false)}
            onSave={handleCorrectionSave}
          />
        )
      }

      {activeTab === 'saves' && (
        <div className="saves-panel">
          {/* Save Locations */}
          <div className="saves-locations">
            <div className="saves-header">
              <span className="saves-title">Save Locations</span>
              {savePathsInfo && (
                <span className={`saves-source-badge saves-source-${savePathsInfo.source}`}>
                  {savePathsInfo.source === 'custom' ? 'Custom' :
                   savePathsInfo.source === 'manifest' ? 'Ludusavi Manifest' :
                   savePathsInfo.source === 'fallback' ? 'Fallback (guessed)' : 'None detected'}
                </span>
              )}
            </div>
            {savePathsInfo && savePathsInfo.paths.length > 0 && (
              <ul className="saves-paths-list">
                {savePathsInfo.paths.map(p => (
                  <li key={p} className="saves-path-item">{p}</li>
                ))}
              </ul>
            )}
            {savePathsInfo && savePathsInfo.paths.length === 0 && (
              <div className="saves-empty">No save paths detected.</div>
            )}
            <div className="saves-custom-row">
              <input
                className="saves-custom-input"
                type="text"
                placeholder="Custom override path (overrides detected paths)"
                value={customPathInput}
                onChange={e => setCustomPathInput(e.target.value)}
              />
              <button
                className="saves-custom-btn"
                onClick={handleSetCustomPath}
                disabled={savingCustomPath}
              >
                {savingCustomPath ? '...' : 'Set'}
              </button>
              {customPathInput && (
                <button
                  className="saves-custom-clear-btn"
                  onClick={() => { setCustomPathInput(''); }}
                  title="Clear"
                >✕</button>
              )}
            </div>
          </div>

          {/* Restore to Device */}
          {agents.filter(a => a.status === 'connected').length > 0 && saveSnapshots.length > 0 && (
            <div style={{ marginTop: '1rem' }}>
              <div className="saves-header">
                <span className="saves-title">Restore Latest to Device</span>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem', marginTop: '0.5rem' }}>
                {agents.filter(a => a.status === 'connected').map(agent => (
                  <button
                    key={agent.id}
                    className="saves-restore-btn"
                    disabled={!!restoringOnAgent[agent.id]}
                    onClick={() => handleRestoreOnAgent(agent.id)}
                    title={`Restore latest save to ${agent.name}`}
                  >
                    {restoringOnAgent[agent.id] ? '...' : `↩ ${agent.name}`}
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Snapshots */}
          <div className="saves-header" style={{ marginTop: '1rem' }}>
            <span className="saves-title">Snapshots</span>
            <button className="saves-refresh-btn" onClick={() => { loadSaveSnapshots(); loadSavePathsInfo(); }} disabled={savesLoading} title="Refresh">
              ↺
            </button>
          </div>
          {savesLoading && <div className="saves-empty">Loading...</div>}
          {!savesLoading && saveSnapshots.length === 0 && (
            <div className="saves-empty">
              No save backups yet. The agent uploads saves automatically when games exit.
            </div>
          )}
          {!savesLoading && saveSnapshots.length > 0 && (
            <div className="saves-table">
              <div className="saves-row saves-row-header">
                <div className="saves-col saves-col-time">Timestamp</div>
                <div className="saves-col saves-col-agent">Device</div>
                <div className="saves-col saves-col-size">Size</div>
                <div className="saves-col saves-col-actions"></div>
              </div>
              {saveSnapshots.map(snap => (
                <div key={snap.id} className="saves-row">
                  <div className="saves-col saves-col-time">
                    {new Date(snap.timestamp).toLocaleString(undefined, {
                      year: 'numeric', month: 'short', day: 'numeric',
                      hour: '2-digit', minute: '2-digit'
                    })}
                  </div>
                  <div className="saves-col saves-col-agent">{snap.agentId.slice(0, 8)}</div>
                  <div className="saves-col saves-col-size">{formatBytes(snap.sizeBytes)}</div>
                  <div className="saves-col saves-col-actions">
                    <button
                      className="saves-delete-btn"
                      title="Delete snapshot"
                      onClick={() => handleDeleteSnapshot(snap.id)}
                    >🗑</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {activeTab === 'files' && (
        <div className="files-panel">
          <div className="files-section">
            <div className="files-section-title" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span>Server Files</span>
              <button className="saves-refresh-btn" onClick={loadServerFiles} disabled={serverFilesLoading} title="Refresh">↺</button>
            </div>
            {game.path && (
              <div style={{ fontSize: '0.75rem', color: '#6c7086', fontFamily: 'monospace', marginBottom: '0.5rem', wordBreak: 'break-all' }}>
                {game.path}
              </div>
            )}
            {serverFilesLoading && <div className="files-empty">Loading...</div>}
            {!serverFilesLoading && serverFiles.length === 0 && (
              <div className="files-empty">
                {game.path ? 'No files found at the game path.' : 'No path set for this game.'}
              </div>
            )}
            {!serverFilesLoading && serverFiles.length > 0 && (
              <div className="files-table">
                <div className="files-row files-row-header">
                  <div className="files-col files-col-path">File</div>
                  <div className="files-col files-col-group">Group</div>
                  <div className="files-col files-col-quality">Quality</div>
                  <div className="files-col files-col-size">Size</div>
                  <div className="files-col files-col-actions"></div>
                </div>
                {serverFiles.map(f => (
                  <div key={f.relativePath} className="files-row">
                    <div className="files-col files-col-path" title={f.relativePath}>{f.name}</div>
                    <div className="files-col files-col-group">{f.releaseGroup || '—'}</div>
                    <div className="files-col files-col-quality">{f.quality || '—'}</div>
                    <div className="files-col files-col-size">{formatBytes(f.size)}</div>
                    <div className="files-col files-col-actions">
                      <button
                        className="saves-delete-btn"
                        title="Delete from server"
                        disabled={!!deletingFile[f.relativePath]}
                        onClick={() => handleDeleteServerFile(f)}
                      >{deletingFile[f.relativePath] ? '⟳' : '✕'}</button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="files-section">
            <div className="files-section-title">Installed On</div>
            {game.installPath && (
              <div className="files-device-row">
                <div className="files-device-info">
                  <div className="files-device-top">
                    <span className="files-device-dot online" />
                    <span className="files-device-name">This server</span>
                  </div>
                  <div className="files-device-meta">
                    <span className="files-device-path" title={game.installPath}>{game.installPath}</span>
                    {game.sizeOnDisk != null && <span className="files-device-size">{formatBytes(game.sizeOnDisk)}</span>}
                    {game.executablePath && <span className="files-device-tag installed">installed</span>}
                  </div>
                </div>
              </div>
            )}
            {(() => {
              const installedAgents = agents.filter(a => a.installedGames?.some(g => agentTitleMatches(g.title, game.title)));
              if (installedAgents.length === 0 && !game.installPath) {
                return <div className="files-empty">Not installed on any registered device.</div>;
              }
              if (installedAgents.length === 0) return null;
              return installedAgents.map(agent => {
                const ig = agent.installedGames!.find(g => agentTitleMatches(g.title, game.title))!;
                const key = `${agent.id}:${ig.installPath}`;
                const isDeleting = deletingFromAgent[key];
                return (
                  <div key={agent.id} className="files-device-row">
                    <div className="files-device-info">
                      <div className="files-device-top">
                        <span className={`files-device-dot ${agent.status}`} />
                        <span className="files-device-name">{agent.name}</span>
                        <span className="files-device-platform">{agent.platform}</span>
                      </div>
                      <div className="files-device-meta">
                        <span className="files-device-path" title={ig.installPath}>{ig.installPath}</span>
                        <span className="files-device-size">{formatBytes(ig.sizeBytes)}</span>
                        {ig.version && <span className="files-device-tag">v{ig.version}</span>}
                        {ig.hasShortcut && <span className="files-device-tag steam">Steam</span>}
                        {(ig.exePath || ig.scriptPath) && <span className="files-device-tag installed">installed</span>}
                      </div>
                      {ig.exeCandidates && ig.exeCandidates.length > 1 && (
                        <div className="files-device-exe-row">
                          <span className="files-device-exe-label">Launch exe:</span>
                          <select
                            className="exe-selector"
                            value={ig.exePath ?? ''}
                            disabled={agent.status !== 'online'}
                            title="Switch which executable run.sh launches"
                            onChange={e => handleChangeExe(agent.id, ig.title, e.target.value)}
                          >
                            {ig.exeCandidates.map(p => (
                              <option key={p} value={p}>{p.split('/').pop() || p.split('\\').pop() || p}</option>
                            ))}
                          </select>
                        </div>
                      )}
                    </div>
                    <div className="files-device-actions">
                      {agent.status === 'online' && (ig.scriptPath || ig.exePath) && (
                        <button
                          className="files-device-btn"
                          title="View launch log"
                          disabled={!!requestingLog[ig.title]}
                          onClick={() => handleViewLog(agent.id, ig.title)}
                        >
                          {requestingLog[ig.title] ? '⟳' : '📋'}
                        </button>
                      )}
                      <button
                        className="files-device-btn"
                        title="Delete from device"
                        disabled={isDeleting}
                        onClick={() => handleDeleteFromAgent(agent.id, ig)}
                      >
                        {isDeleting ? '⟳' : '✕'}
                      </button>
                    </div>
                  </div>
                );
              });
            })()}
          </div>
        </div>
      )}

      {logModal && (
        <div className="modal-overlay" onClick={() => setLogModal(null)}>
          <div className="modal" style={{ maxWidth: '780px', width: '90vw' }} onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Launch Log — {logModal.gameTitle}</h3>
              <button className="modal-close" onClick={() => setLogModal(null)}>×</button>
            </div>
            <div className="modal-content">
              <div style={{ fontSize: '0.75rem', color: '#a6adc8', marginBottom: '0.5rem' }}>
                {logModal.agentName} · ~/Games/{logModal.gameTitle}/run.log
              </div>
              <pre style={{
                background: '#11111b',
                color: '#cdd6f4',
                padding: '1rem',
                borderRadius: '6px',
                fontSize: '0.72rem',
                lineHeight: '1.5',
                overflowY: 'auto',
                maxHeight: '60vh',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                fontFamily: 'monospace',
              }}>
                {logModal.content}
              </pre>
            </div>
          </div>
        </div>
      )}

      {showExePicker && (
        <div className="modal-overlay" onClick={() => setShowExePicker(false)}>
          <div className="modal exe-picker-modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Choose Executable</h3>
              <button className="modal-close" onClick={() => setShowExePicker(false)}>✕</button>
            </div>
            <div className="modal-content">
              <p className="exe-picker-hint">Multiple executables found — select which one to use:</p>
              <div className="exe-picker-list">
                {exeCandidates.map(c => (
                  <button
                    key={c.relPath}
                    className={`exe-picker-item${selectedExeName === c.name ? ' selected' : ''}`}
                    onClick={() => setSelectedExeName(c.name)}
                  >
                    <span className={`exe-type-badge ${c.type}`}>
                      {c.type === 'installer' ? 'Installer' : 'Game'}
                    </span>
                    <div className="exe-info">
                      <span className="exe-name">{c.name}</span>
                      <span className="exe-path">
                        {c.fromArchive ? `inside ${c.fromArchive} · ` : ''}{c.relPath}
                      </span>
                    </div>
                  </button>
                ))}
              </div>
            </div>
            <div className="modal-actions">
              <button onClick={() => { setShowExePicker(false); setPendingInstall(null); }}>Cancel</button>
              <button
                className="action-btn install-ready"
                disabled={!selectedExeName}
                onClick={async () => {
                  setShowExePicker(false);
                  if (pendingInstall) {
                    await dispatchInstall(pendingInstall.agentId, pendingInstall.installDir, selectedExeName);
                    setPendingInstall(null);
                  }
                }}
              >
                Install
              </button>
            </div>
          </div>
        </div>
      )}

      {
        showInstallWarning && (
          <div className="modal-overlay">
            <div className="modal">
              <div className="modal-header">
                <h3>{t('installWarningTitle')}</h3>
                <button className="modal-close" onClick={() => setShowInstallWarning(false)}>×</button>
              </div>
              <div className="modal-content">
                <p style={{ color: '#cdd6f4', lineHeight: '1.6', marginBottom: '1.5rem' }}>{t('installWarningBody')}</p>
                <div className="modal-actions">
                  <button
                    className="btn-secondary"
                    onClick={() => setShowInstallWarning(false)}
                  >
                    {t('cancel')}
                  </button>
                  <button
                    className="btn-danger"
                    onClick={() => onWarningConfirmed()}
                  >
                    {t('confirmInstall')}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )
      }
      {
        showVersionModal && (
          <VersionSelectorModal
            isOpen={showVersionModal}
            onClose={() => setShowVersionModal(false)}
            onSelect={(path) => {
              if (actionType === 'install') executeInstall(path);
              else executePlay(path);
            }}
            options={versionOptions}
            gameTitle={game?.title || 'Game'}
          />
        )
      }
    </div >
  );
};

export default GameDetails;
