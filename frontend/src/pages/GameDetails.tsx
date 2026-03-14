import React, { useEffect, useState, useMemo } from 'react';
import { useParams, Link } from 'react-router-dom';
import axios from 'axios';
import { authFetch } from '../App';
import { t } from '../i18n/translations';
import { useSearchCache } from '../context/SearchCacheContext';
import GameCorrectionModal from '../components/GameCorrectionModal';
import UninstallModal from '../components/UninstallModal';
import SwitchInstallerModal from '../components/SwitchInstallerModal';
import AgentFileBrowserModal from '../components/AgentFileBrowserModal';
import WorkflowPipeline, { WorkflowStep } from '../components/WorkflowPipeline';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faPen, faFolderOpen, faDownload, faMagnet, faSpinner, faSort, faSortUp, faSortDown, faArrowUp, faArrowDown, faTrash, faMicrochip, faDatabase, faArrowsRotate, faFolder, faFile, faCheck } from '@fortawesome/free-solid-svg-icons';
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
  isInstallable?: boolean;
  availablePlatforms?: string[];
  steamId?: number;
  igdbId?: number;
  path?: string;
  sizeOnDisk?: number;
  downloadPath?: string;
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
  source: 'manifest' | 'prefix-scan' | 'fallback' | 'none';
  paths: string[];         // merged (custom + detected), for agent use
  customPaths: string[];   // only user-added paths (subset of paths)
  agentPaths: Record<string, string[]>; // agentId → custom paths
  steamId?: number;
}

interface DownloadItem {
  clientId: number;
  id: string;
  name: string;
  size: number;
  progress: number;
  state: number; // 0=Downloading, 1=Paused, 2=Completed, 3=Error, 4=Queued, 5=Checking
  category: string;
  downloadPath: string;
  clientName: string;
  infoHash?: string;
}

function extractInfoHash(magnetUrl: string): string {
  const m = magnetUrl.match(/xt=urn:bt[im]h:([A-Fa-f0-9]{40}|[A-Za-z2-7]{32})/i);
  return m ? m[1].toLowerCase() : '';
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
  // Scorer fields — populated by backend
  score?: number;
  scorePct?: number;
  titleScore?: number;
  sourceName?: string;
  sourceTier?: number;
  detectedVersion?: string;
  detectedLangs?: string[];
  releaseType?: string;      // "game" | "update" | "dlc" | "demo"
  installMethod?: string;    // "DirectExtract" | "RunInstaller" | "Unknown"
  sizeWarning?: string;      // "too_large" | ""
  sizeOutlier?: boolean;     // true when size is a clear outlier vs other results
}

function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + '\u00a0' + sizes[i];
}

// ── File tree ──────────────────────────────────────────────
interface FileTreeNode {
  name: string;
  isDir: boolean;
  children: FileTreeNode[];
  // file-only
  relativePath?: string;
  size?: number;
  gameFileId?: number;
  quality?: string;
  releaseGroup?: string;
}

function buildFileTree(files: ServerFile[]): FileTreeNode[] {
  const root: FileTreeNode = { name: '', isDir: true, children: [] };
  for (const f of files) {
    const parts = f.relativePath.split('/');
    let node = root;
    for (let i = 0; i < parts.length - 1; i++) {
      let dir = node.children.find(c => c.isDir && c.name === parts[i]);
      if (!dir) {
        dir = { name: parts[i], isDir: true, children: [] };
        node.children.push(dir);
      }
      node = dir;
    }
    node.children.push({
      name: f.name,
      isDir: false,
      children: [],
      relativePath: f.relativePath,
      size: f.size,
      gameFileId: f.gameFileId,
      quality: f.quality,
      releaseGroup: f.releaseGroup,
    });
  }
  return root.children;
}

interface FileTreeRowProps {
  node: FileTreeNode;
  depth: number;
  deletingFile: Record<string, boolean>;
  onDelete: (file: ServerFile) => void;
}

const FileTreeRow: React.FC<FileTreeRowProps> = ({ node, depth, deletingFile, onDelete }) => {
  const [open, setOpen] = React.useState(false);
  const indent = depth * 16;

  if (node.isDir) {
    return (
      <>
        <div
          className="gd-tree-row gd-tree-dir"
          style={{ paddingLeft: `${6 + indent}px` }}
          onClick={() => setOpen(o => !o)}
        >
          <span className="gd-tree-arrow">{open ? '▾' : '▸'}</span>
          <FontAwesomeIcon icon={faFolder} className="gd-tree-folder-icon" />
          <span className="gd-tree-name">{node.name}</span>
        </div>
        {open && node.children.map((child, i) => (
          <FileTreeRow
            key={child.isDir ? `d:${child.name}:${i}` : child.relativePath}
            node={child}
            depth={depth + 1}
            deletingFile={deletingFile}
            onDelete={onDelete}
          />
        ))}
      </>
    );
  }

  const serverFile: ServerFile = {
    relativePath: node.relativePath!,
    name: node.name,
    size: node.size!,
    gameFileId: node.gameFileId,
    quality: node.quality,
    releaseGroup: node.releaseGroup,
  };

  return (
    <div className="gd-tree-row gd-tree-file" style={{ paddingLeft: `${6 + indent}px` }}>
      <FontAwesomeIcon icon={faFile} className="gd-tree-file-icon" />
      <span className="gd-tree-name" title={node.relativePath}>{node.name}</span>
      <span className="gd-tree-meta">{node.releaseGroup || ''}</span>
      <span className="gd-tree-meta">{node.quality || ''}</span>
      <span className="gd-tree-size">{formatBytes(node.size!)}</span>
      <button
        className="gd-icon-btn gd-icon-btn-danger gd-tree-del"
        title="Delete from server"
        disabled={!!deletingFile[node.relativePath!]}
        onClick={() => onDelete(serverFile)}
      >{deletingFile[node.relativePath!] ? '⟳' : '✕'}</button>
    </div>
  );
};

const GameDetails: React.FC = () => {
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
  const [showSwitchModal, setShowSwitchModal] = useState(false);
  const [showSearchModal, setShowSearchModal] = useState(false);
  const [serverFiles, setServerFiles] = useState<ServerFile[]>([]);
  const [serverFilesLoading, setServerFilesLoading] = useState(false);
  const [deletingFile, setDeletingFile] = useState<Record<string, boolean>>({});
  const [manifestInfo, setManifestInfo] = useState<{ appId: number; depots: { depotId: number; manifestGid: string }[] } | null>(null);
  const [manifestFetching, setManifestFetching] = useState(false);
  const [manifestUploadRef] = useState(() => React.createRef<HTMLInputElement>());
  const [steamDownloading, setSteamDownloading] = useState(false);
  const [saveSnapshots, setSaveSnapshots] = useState<SaveSnapshot[]>([]);
  const [savesLoading, setSavesLoading] = useState(false);
  const [savePathsInfo, setSavePathsInfo] = useState<SavePathsInfo | null>(null);
  const [browserAgent, setBrowserAgent] = useState<{ id: string; name: string; initialPath?: string } | null>(null);
  const [saveConflict, setSaveConflict] = useState<{ gameId: number; title: string; uploadingAgentId: string; conflictingAgentId: string } | null>(null);
  const [hasSearched, setHasSearched] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [updateBannerDismissed, setUpdateBannerDismissed] = useState(false);
  const [downloadQueue, setDownloadQueue] = useState<DownloadItem[]>([]);
  const [importRunning, setImportRunning] = useState(false);

  const queuedInfoHashes = useMemo(() => {
    const s = new Set<string>();
    for (const item of downloadQueue) {
      if (item.infoHash) s.add(item.infoHash.toLowerCase());
    }
    return s;
  }, [downloadQueue]);
  const [refreshingMeta, setRefreshingMeta] = useState(false);

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

  // Per-device launch settings (inline in device cards)
  interface ProtonVersionInfo { name: string; binPath: string; }
  const [agentLaunchArgs, setAgentLaunchArgs] = useState<Record<string, string>>({});
  const [agentEnvVars, setAgentEnvVars] = useState<Record<string, string>>({});
  const [agentProtonPath, setAgentProtonPath] = useState<Record<string, string>>({});
  const [agentProtonVersions, setAgentProtonVersions] = useState<Record<string, ProtonVersionInfo[]>>({});
  const [savedAgentSettings, setSavedAgentSettings] = useState<Record<string, { launchArgs: string; envVars: string; protonPath: string }>>({});
  const [savingLaunchSettings, setSavingLaunchSettings] = useState<Record<string, boolean>>({});
  const [expandedLaunchConfig, setExpandedLaunchConfig] = useState<Record<string, boolean>>({});
  const [requestingScript, setRequestingScript] = useState<Record<string, boolean>>({});
  const [scriptModal, setScriptModal] = useState<{ agentName: string; gameTitle: string; content: string } | null>(null);
  const pendingScriptRequest = React.useRef<string | null>(null);

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

  // Listen for save conflicts on this game
  useEffect(() => {
    const handler = (e: Event) => {
      try {
        const data = JSON.parse((e as CustomEvent).detail);
        if (game && data.gameId === game.id) {
          setSaveConflict(data);
        }
      } catch { /* ignore */ }
    };
    window.addEventListener('SAVE_CONFLICT_EVENT', handler);
    return () => window.removeEventListener('SAVE_CONFLICT_EVENT', handler);
  }, [game?.id]);

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

  // Listen for script content from agent
  useEffect(() => {
    const handler = (e: Event) => {
      try {
        const payload = JSON.parse((e as CustomEvent).detail);
        if (payload.requestId && payload.requestId === pendingScriptRequest.current) {
          const agentName = agents.find(a => a.installedGames?.some(g => agentTitleMatches(g.title, payload.gameTitle)))?.name ?? 'Agent';
          setScriptModal({ agentName, gameTitle: payload.gameTitle, content: payload.content });
          setRequestingScript(prev => { const next = { ...prev }; delete next[payload.gameTitle]; return next; });
          pendingScriptRequest.current = null;
        }
      } catch { /* ignore */ }
    };
    window.addEventListener('AGENT_SCRIPT_DATA_EVENT', handler);
    return () => window.removeEventListener('AGENT_SCRIPT_DATA_EVENT', handler);
  }, [agents]);

  // Reset per-device UI state when navigating to a different game
  useEffect(() => {
    if (!id) return;
    setExpandedLaunchConfig({});
    setAgentProtonVersions({});
    setSavingLaunchSettings({});
    pendingScriptRequest.current = null;
  }, [id]);

  // Load per-device launch args when game changes
  useEffect(() => {
    if (!id) return;
    axios.get(`/api/v3/game/${id}/agent-launch-args`).then(res => {
      const settings: Record<string, { launchArgs: string; envVars: string; protonPath: string }> = res.data || {};
      const launchArgs: Record<string, string> = {};
      const envVars: Record<string, string> = {};
      const protonPaths: Record<string, string> = {};
      Object.entries(settings).forEach(([agId, s]) => {
        launchArgs[agId] = s.launchArgs || '';
        envVars[agId] = s.envVars || '';
        protonPaths[agId] = s.protonPath || '';
      });
      setAgentLaunchArgs(launchArgs);
      setAgentEnvVars(envVars);
      setAgentProtonPath(protonPaths);
      setSavedAgentSettings(settings);
    }).catch(() => {});
  }, [id]);

  // Load proton versions when a device's launch config is expanded
  useEffect(() => {
    Object.entries(expandedLaunchConfig).forEach(([agentId, expanded]) => {
      if (!expanded || agentProtonVersions[agentId] !== undefined) return;
      const agent = agents.find(a => a.id === agentId);
      if (!agent || agent.status !== 'online') return;
      axios.get(`/api/v3/agent/${agentId}/proton-versions`)
        .then(res => setAgentProtonVersions(prev => ({ ...prev, [agentId]: res.data || [] })))
        .catch(() => setAgentProtonVersions(prev => ({ ...prev, [agentId]: [] })));
    });
  }, [expandedLaunchConfig]); // eslint-disable-line react-hooks/exhaustive-deps

  // ---- Download queue ----
  useEffect(() => {
    axios.get<DownloadItem[]>('/api/v3/downloadclient/queue').then(r => setDownloadQueue(r.data || [])).catch(() => {});
    const handler = (e: Event) => {
      try { setDownloadQueue(JSON.parse((e as CustomEvent).detail) || []); } catch { /* ignore */ }
    };
    window.addEventListener('DOWNLOAD_QUEUE_UPDATED_EVENT', handler);
    return () => window.removeEventListener('DOWNLOAD_QUEUE_UPDATED_EVENT', handler);
  }, []);

  // Clear importRunning when library is updated
  useEffect(() => {
    const handler = () => {
      setImportRunning(false);
      if (id) {
        axios.get(`/api/v3/game/${id}`).then(r => setGame(r.data)).catch(() => {});
      }
    };
    window.addEventListener('LIBRARY_UPDATED_EVENT', handler);
    return () => window.removeEventListener('LIBRARY_UPDATED_EVENT', handler);
  }, [id]);

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

  // ---- Import (post-processor) ----
  const handleImport = async () => {
    if (!id) return;
    setImportRunning(true);
    try {
      await axios.post(`/api/v3/game/${id}/import`);
      setNotification({ message: 'Import started', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.message || 'Import failed', type: 'error' });
    } finally {
      setImportRunning(false);
    }
  };

  // ---- Workflow pipeline steps ----
  const buildWorkflowSteps = (): WorkflowStep[] => {
    if (!game) return [];

    const titleLower = game.title.toLowerCase();
    const matchingDownload = downloadQueue.find(d => {
      const nameLower = d.name.toLowerCase();
      return nameLower.includes(titleLower) ||
        titleLower.includes(nameLower.split(' ').slice(0, 3).join(' '));
    });

    const anyAgentHasGame = agents.some(a =>
      a.installedGames?.some(ig => agentTitleMatches(ig.title, game.title))
    );
    const anyAgentInstalled = agents.some(a =>
      a.installedGames?.some(ig => agentTitleMatches(ig.title, game.title) && ig.exePath)
    );

    const jobStatus = agentJobProgress?.status ?? '';

    // Step 1: Library — always done
    const step1: WorkflowStep = {
      id: 'library',
      label: 'Library',
      status: 'done',
    };

    // Step 2: Find Download
    const hasDownload = !!matchingDownload || !!game.path;
    const step2: WorkflowStep = {
      id: 'find-download',
      label: 'Find Download',
      status: hasDownload ? 'done' : 'pending',
    };

    // Step 3: Downloading
    let step3Status: WorkflowStep['status'] = 'pending';
    let step3Detail: string | undefined;
    if (game.path || matchingDownload?.state === 2) {
      step3Status = 'done';
    } else if (matchingDownload && (matchingDownload.state === 0 || matchingDownload.state === 4)) {
      step3Status = 'active';
      step3Detail = `${Math.round(matchingDownload.progress)}%`;
    }
    const step3: WorkflowStep = {
      id: 'downloading',
      label: 'Downloading',
      status: step3Status,
      detail: step3Detail,
    };

    // Step 4: Game Folder
    const hasFolderFiles = !!game.path;
    let step4Status: WorkflowStep['status'] = 'pending';
    if (hasFolderFiles) {
      step4Status = 'done';
    } else if (importRunning) {
      step4Status = 'active';
    }
    const canImport = !!(matchingDownload?.state === 2 && !game.path) || !!(matchingDownload?.downloadPath && !game.path);
    const step4: WorkflowStep = {
      id: 'game-folder',
      label: 'Game Folder',
      status: step4Status,
      detail: game.path ? game.path.split('/').pop() : undefined,
      action: canImport ? {
        label: 'Move to Library',
        onClick: handleImport,
        disabled: importRunning,
      } : undefined,
    };

    // Step 5: On Device
    let step5Status: WorkflowStep['status'] = 'pending';
    if (anyAgentHasGame) {
      step5Status = 'done';
    } else if (jobStatus === 'downloading' || jobStatus === 'extracting') {
      step5Status = 'active';
    }
    const onlineAgents = agents.filter(a => a.status === 'online');
    const step5: WorkflowStep = {
      id: 'on-device',
      label: 'On Device',
      status: step5Status,
      action: hasFolderFiles && onlineAgents.length > 0 && !anyAgentHasGame ? {
        label: 'Install ▾',
        onClick: () => setShowAgentDropdown(v => !v),
      } : undefined,
    };

    // Step 6: Installed
    let step6Status: WorkflowStep['status'] = 'pending';
    if (anyAgentInstalled) {
      step6Status = 'done';
    } else if (jobStatus === 'installing' || jobStatus === 'creating_shortcut') {
      step6Status = 'active';
    }
    const step6: WorkflowStep = {
      id: 'installed',
      label: 'Installed',
      status: step6Status,
    };

    return [step1, step2, step3, step4, step5, step6];
  };

  useEffect(() => {
    const loadGame = async () => {
      if (!id) return;
      try {
        const response = await axios.get(`/api/v3/game/${id}?lang=en`);
        setGame(response.data);
        setSearchTerm(prev => prev || response.data.title);
      } catch (err: any) {
        setError(err.response?.data?.message || t('error'));
      } finally {
        setLoading(false);
      }
    };

    loadGame();
  }, [id]);


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
        protocol: protocol,
        gameId: game?.id
      });
      setNotification({ message: response.data.message || t('downloadStarted'), type: 'success' });
    } catch (error: any) {
      console.error('Download failed:', error);
      const errorMessage = error.response?.data?.error || error.response?.data?.message || t('failedToDownload');
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

  const scoreColor = (pct: number) =>
    pct >= 70 ? '#4ade80' : pct >= 40 ? '#fbbf24' : '#f87171';

  // Reconstruct component scores to show in the tooltip (mirrors scorer.go logic).
  const scoreBreakdown = (r: TorrentResult) => {
    const titlePts = Math.round((r.titleScore ?? 0) * 0.30);
    const sourcePts = r.sourceTier === 1 ? 40 : r.sourceTier === 2 ? 30 : r.sourceTier === 3 ? 20 : 0;
    const proto = (r.protocol ?? '').toLowerCase();
    const s = r.seeders ?? 0;
    const seederPts = proto === 'nzb' ? 20 : s === 0 ? 0 : s < 5 ? 5 : s < 20 ? 10 : s < 100 ? 15 : 20;
    const sizeOK = !r.sizeWarning && !r.sizeOutlier;
    const sizePts = sizeOK ? 10 : 0;
    const sourceDesc = r.sourceTier === 1 ? 'official' : r.sourceTier === 2 ? 'repacker' : r.sourceTier === 3 ? 'scene' : 'unknown';
    return { titlePts, sourcePts, seederPts, sizePts, sourceDesc };
  };

  // Flag results whose size is a clear outlier compared to the rest of the set.
  // Only compares results of the same release type to avoid comparing updates to full games.
  const flagSizeOutliers = (items: TorrentResult[]): TorrentResult[] => {
    const byType: Record<string, number[]> = {};
    for (const r of items) {
      if (r.size <= 0) continue;
      const t = r.releaseType ?? 'game';
      (byType[t] = byType[t] ?? []).push(r.size);
    }
    const medians: Record<string, number> = {};
    for (const [t, sizes] of Object.entries(byType)) {
      if (sizes.length < 4) continue; // not enough data to judge
      const sorted = [...sizes].sort((a, b) => a - b);
      medians[t] = sorted[Math.floor(sorted.length / 2)];
    }
    return items.map(r => {
      const median = medians[r.releaseType ?? 'game'];
      const outlier = median != null && r.size > 0 && r.size < median * 0.2;
      return outlier === r.sizeOutlier ? r : { ...r, sizeOutlier: outlier };
    });
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
    } catch {
      setSavePathsInfo(null);
    }
  };

  const loadManifestInfo = async (gameId: number) => {
    try {
      const r = await axios.get(`/api/v3/game/${gameId}/steam-manifest-info`);
      setManifestInfo(r.data);
    } catch {
      setManifestInfo(null);
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

  const handleFetchManifest = async () => {
    if (!game) return;
    setManifestFetching(true);
    try {
      const r = await axios.post(`/api/v3/game/${game.id}/fetch-manifest`);
      setManifestInfo(r.data);
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Manifest fetch failed', type: 'error' });
    }
    setManifestFetching(false);
  };

  const handleManifestUpload = async (file: File) => {
    if (!game) return;
    const form = new FormData();
    form.append('file', file);
    try {
      const r = await axios.post(`/api/v3/game/${game.id}/steam-manifest`, form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      setManifestInfo(r.data);
      setNotification({ message: 'Manifest uploaded', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Upload failed', type: 'error' });
    }
  };

  const handleSteamDownload = async () => {
    if (!game) return;
    setSteamDownloading(true);
    try {
      const r = await axios.post(`/api/v3/game/${game.id}/steam-download`);
      setNotification({ message: `Steam download started (job ${r.data.jobId})`, type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Failed to start Steam download', type: 'error' });
    }
    setSteamDownloading(false);
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

  const handleRenamePrefix = async (agentId: string) => {
    if (!game) return;
    try {
      await axios.post(`/api/v3/agent/${agentId}/rename-prefix`, { gameId: game.id, gameTitle: game.title });
      setNotification({ message: 'Prefix rename requested — the agent will rename the directory and update run.sh.', type: 'success' });
    } catch (err: any) {
      setNotification({ message: 'Rename failed: ' + (err.response?.data?.error || err.message), type: 'error' });
    }
  };

  const handleSaveLaunchSettings = async (agentId: string) => {
    if (!id) return;
    setSavingLaunchSettings(prev => ({ ...prev, [agentId]: true }));
    try {
      await axios.patch(`/api/v3/game/${id}/agent-launch-args`, {
        agentId,
        launchArgs: agentLaunchArgs[agentId] ?? '',
        envVars: agentEnvVars[agentId] ?? '',
        protonPath: agentProtonPath[agentId] ?? '',
      });
      setSavedAgentSettings(prev => ({
        ...prev,
        [agentId]: { launchArgs: agentLaunchArgs[agentId] ?? '', envVars: agentEnvVars[agentId] ?? '', protonPath: agentProtonPath[agentId] ?? '' },
      }));
      setNotification({ message: 'Launch settings saved', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Failed to save launch settings', type: 'error' });
    } finally {
      setSavingLaunchSettings(prev => ({ ...prev, [agentId]: false }));
    }
  };

  const handleRestartSteam = async (agentId: string) => {
    try {
      await axios.post(`/api/v3/agent/${agentId}/restart-steam`);
      setNotification({ message: 'Steam restart requested', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Failed to restart Steam', type: 'error' });
    }
  };

  const handleReloadScript = async (agentId: string) => {
    try {
      await axios.post(`/api/v3/agent/${agentId}/regen-scripts`);
      setNotification({ message: 'Script reload requested', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Failed to reload scripts', type: 'error' });
    }
  };

  const handleViewScript = async (agentId: string, gameTitle: string) => {
    setRequestingScript(prev => ({ ...prev, [gameTitle]: true }));
    try {
      const r = await axios.post<{ requestId: string }>(`/api/v3/agent/${agentId}/readscript`, { gameTitle });
      pendingScriptRequest.current = r.data.requestId;
    } catch (err: any) {
      alert(`Failed to request script: ${err.response?.data?.error || err.message}`);
      setRequestingScript(prev => { const next = { ...prev }; delete next[gameTitle]; return next; });
    }
  };

  const handleReloadShortcut = async (agentId: string) => {
    try {
      await axios.post(`/api/v3/agent/${agentId}/refresh-shortcuts`);
      setNotification({ message: 'Shortcut reload requested', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Failed to reload shortcut', type: 'error' });
    }
  };

  const handleRefreshAgent = async (agentId: string) => {
    try {
      await axios.post(`/api/v3/agent/${agentId}/scan`);
      setNotification({ message: 'Agent scan requested', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.response?.data?.error || 'Failed to scan agent', type: 'error' });
    }
  };

  const handleKeepSaves = async (keepAgentId: string, discardAgentId: string) => {
    if (!game || !saveConflict) return;
    try {
      // Promote the keeper's snapshot as the new latest, then restore to all agents
      await axios.post(`/api/v3/save/${game.id}/promote-snapshot?sourceAgentId=${keepAgentId}`);
      const keepName = agents.find(a => a.id === keepAgentId)?.name ?? keepAgentId.slice(0, 8);
      setNotification({ message: `Restoring ${keepName}'s saves to all devices.`, type: 'success' });
      setSaveConflict(null);
      setTimeout(() => { loadSaveSnapshots(); }, 1000);
    } catch (err: any) {
      setNotification({ message: 'Conflict resolution failed: ' + (err.response?.data?.error || err.message), type: 'error' });
    }
  };

  const handleBrowseConfirm = async (agentId: string, paths: string[]) => {
    if (!game) return;
    try {
      await axios.patch(`/api/v3/save/${game.id}/path`, { savePaths: paths, agentId });
      setBrowserAgent(null);
      await loadSavePathsInfo();
      setNotification({ message: `Save path${paths.length > 1 ? 's' : ''} updated.`, type: 'success' });
    } catch (err: any) {
      setNotification({ message: 'Failed to set path: ' + (err.response?.data?.error || err.message), type: 'error' });
    }
  };

  const handleRemoveAgentPath = async (agentId: string) => {
    if (!game) return;
    try {
      await axios.patch(`/api/v3/save/${game.id}/path`, { savePath: '', agentId });
      await loadSavePathsInfo();
    } catch (err: any) {
      setNotification({ message: 'Failed to remove path: ' + (err.response?.data?.error || err.message), type: 'error' });
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

  const handleUploadFromAgent = async (agentId: string) => {
    if (!game) return;
    try {
      await axios.post(`/api/v3/agent/${agentId}/upload-save`, { title: game.title });
      setNotification({ message: 'Upload requested — saves will appear in Snapshots shortly.', type: 'success' });
      setTimeout(() => loadSaveSnapshots(), 5000);
    } catch (err: any) {
      setNotification({ message: 'Upload failed: ' + (err.response?.data?.error || err.message), type: 'error' });
    }
  };

  // Eagerly load files and saves when game data is first available
  useEffect(() => {
    if (!game) return;
    loadServerFiles();
    loadSaveSnapshots();
    loadSavePathsInfo();
    loadManifestInfo(game.id);
  }, [game?.id]); // eslint-disable-line react-hooks/exhaustive-deps

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

    const accumulated: TorrentResult[] = [];
    try {
      const params = new URLSearchParams({ query });
      if (cats) params.set('categories', cats);
      const response = await authFetch(`/api/v3/search?${params}`);
      if (!response.ok || !response.body) {
        throw new Error(`Search failed: ${response.status}`);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? ''; // keep incomplete last line

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed) continue;
          try {
            const batch: TorrentResult[] = JSON.parse(trimmed);
            accumulated.push(...batch);
            // Re-sort by score descending after each batch, then flag outliers
            accumulated.sort((a, b) => (b.scorePct ?? 0) - (a.scorePct ?? 0));
            setResults(flagSizeOutliers(accumulated));
            setHasSearched(true);
            setSearching(false); // show results immediately; spinner goes away after first batch
          } catch { /* malformed line, skip */ }
        }
      }

      if (id) {
        setCacheForGame(parseInt(id), accumulated);
      }
    } catch (err: any) {
      setError(err.message || t('error'));
    } finally {
      setSearching(false);
      setHasSearched(true);
    }
  };

  const refreshMetadata = async () => {
    if (!game || refreshingMeta) return;
    setRefreshingMeta(true);
    try {
      const res = await authFetch(`/api/v3/game/${game.id}/refresh-metadata`, { method: 'POST' });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.message || 'Refresh failed');
      }
      const updated = await res.json();
      setGame(updated);
      setNotification({ message: 'Metadata refreshed from IGDB', type: 'success' });
    } catch (err: any) {
      setNotification({ message: err.message || 'Failed to refresh metadata', type: 'error' });
    } finally {
      setRefreshingMeta(false);
    }
  };

  const handleCorrectionSave = async (updates: any) => {
    if (!game) return;
    try {
      await axios.put(`/api/v3/game/${game.id}`, updates);
      setNotification({ message: t('gameUpdated'), type: 'success' });
      setShowCorrectionModal(false);
      // Reload game to reflect changes
      const response = await axios.get(`/api/v3/game/${game.id}?lang=en`);
      setGame(response.data);
    } catch (err: any) {
      console.error(err);
      setNotification({ message: t('errorUpdating'), type: 'error' });
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
      {/* Cinematic backdrop */}
      {(game.images.backgroundUrl || (game.images.screenshots && game.images.screenshots.length > 0)) && (
        <div
          className="gd-backdrop"
          style={{ backgroundImage: `url(${game.images.backgroundUrl || game.images.screenshots![0]})` }}
        />
      )}

      {/* Toast notification */}
      {notification && (
        <div className={`gd-toast gd-toast-${notification.type}`}>
          {notification.message}
        </div>
      )}

      {/* Breadcrumb */}
      <div className="gd-breadcrumb">
        <Link to="/library" className="gd-breadcrumb-back">← {t('library')}</Link>
        <span className="gd-breadcrumb-sep">/</span>
        <span className="gd-breadcrumb-current">{game.title}</span>
      </div>
      {/* Hero section */}
      <div className="gd-hero">
        <div className="gd-cover-wrap">
          {game.images.coverUrl ? (
            <img className="gd-cover-img" src={game.images.coverUrl} alt={game.title} />
          ) : (
            <div className="gd-cover-placeholder">?</div>
          )}
        </div>
        <div className="gd-hero-body">
          <h1 className="gd-title">{game.title}</h1>
          <div className="gd-meta-row">
            {game.year && <span className="gd-meta-pill">{game.year}</span>}
            {game.platform && <span className="gd-meta-pill">{game.platform.name}</span>}
            {game.rating && <span className="gd-meta-pill">★ {Math.round(game.rating)}%</span>}
          </div>
          {game.genres && game.genres.length > 0 && (
            <div className="gd-genres">{game.genres.join(' · ')}</div>
          )}
          {game.availablePlatforms && game.availablePlatforms.length > 0 && (
            <div className="gd-platforms">
              {game.availablePlatforms.map(p => (
                <span key={p} className="gd-platform-tag">{p}</span>
              ))}
            </div>
          )}
          {game.overview && <p className="gd-overview">{game.overview}</p>}

          {/* Action toolbar */}
          <div className="gd-actions">
            <button
              className="gd-btn gd-btn-primary"
              onClick={() => { setShowSearchModal(true); if (!hasSearched) handleSearchTorrents(); }}
            >
              <FontAwesomeIcon icon={faSearch} /> Search Downloads
            </button>
            <button className="gd-btn" onClick={() => setShowCorrectionModal(true)}>
              <FontAwesomeIcon icon={faPen} /> Correct Metadata
            </button>
            {game.igdbId && (
              <button className="gd-btn" onClick={refreshMetadata} disabled={refreshingMeta}>
                <FontAwesomeIcon icon={faArrowsRotate} spin={refreshingMeta} /> {refreshingMeta ? 'Refreshing…' : 'Refresh Metadata'}
              </button>
            )}
            <button className="gd-btn gd-btn-danger" onClick={() => setShowUninstallModal(true)}>
              <FontAwesomeIcon icon={faTrash} /> Remove
            </button>

            {!isSwitchGame && (
              <div className="gd-install-wrap">
                <button
                  className={`gd-btn${game.isInstallable ? ' gd-btn-install' : ''}`}
                  onClick={() => setShowAgentDropdown(v => !v)}
                >
                  <FontAwesomeIcon icon={faDownload} /> {t('install')} ▾
                </button>
                {showAgentDropdown && (
                  <div className="gd-agent-dropdown">
                    {agents.length === 0 && (
                      <div className="gd-dropdown-label gd-dropdown-no-agents">No agents registered</div>
                    )}
                    {agents.map(a => {
                      const isOnline = a.status === 'online';
                      const isInstalled = a.installedGames?.some(g => agentTitleMatches(g.title, game.title ?? '')) ?? false;
                      const paths = a.installPaths || [];
                      if (isOnline && paths.length > 1) {
                        return (
                          <React.Fragment key={a.id}>
                            <div className="gd-dropdown-label">
                              📱 {a.name}
                              {isInstalled && <span className="gd-dropdown-installed-badge">installed</span>}
                            </div>
                            {paths.map((p, i) => (
                              <button key={i} className="gd-dropdown-item gd-dropdown-item-indented"
                                onClick={() => handleRemoteInstall(a.id, p.path)}>
                                <span>› {p.label}</span>
                                {p.freeBytes >= 0 && <span className="gd-dropdown-free">{formatFreeSpace(p.freeBytes)}</span>}
                              </button>
                            ))}
                          </React.Fragment>
                        );
                      }
                      return (
                        <button
                          key={a.id}
                          className={`gd-dropdown-item${!isOnline ? ' gd-dropdown-item-offline' : ''}`}
                          onClick={() => isOnline && handleRemoteInstall(a.id, paths[0]?.path)}
                          disabled={!isOnline}
                          title={!isOnline ? `${a.name} is offline` : undefined}
                        >
                          <span className="gd-dropdown-agent-name">
                            <span className={`gd-agent-dot ${isOnline ? 'online' : 'offline'}`} />
                            {a.name}
                          </span>
                          {isInstalled && <span className="gd-dropdown-installed-badge">installed</span>}
                          {!isOnline && <span className="gd-dropdown-offline-label">offline</span>}
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>
            )}

            {isSwitchGame && (
              <button className="gd-btn gd-btn-switch" onClick={() => setShowSwitchModal(true)}>
                <FontAwesomeIcon icon={faMicrochip} /> USB Install
              </button>
            )}
          </div>

          {/* Agent install progress */}
          {agentJobProgress && (
            <div className="gd-install-progress">
              <div className="gd-install-progress-header">
                <span>{agentJobProgress.message}</span>
                <span>{agentJobProgress.percent}%</span>
              </div>
              <div className="gd-install-progress-bar">
                <div
                  className="gd-install-progress-fill"
                  style={{
                    width: `${agentJobProgress.percent}%`,
                    background: agentJobProgress.status === 'done' ? '#a6e3a1'
                      : agentJobProgress.status === 'failed' ? '#f38ba8' : '#89b4fa',
                  }}
                />
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Workflow pipeline */}
      <WorkflowPipeline steps={buildWorkflowSteps()} />

      {/* Update banner */}
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
              onClick={() => { setShowSearchModal(true); handleSearchTorrents(game.title); }}
            >
              Search Now
            </button>
            <button className="update-banner-btn" onClick={() => setUpdateBannerDismissed(true)}>
              Dismiss
            </button>
          </div>
        </div>
      )}

      {/* ── Content grid: Files+Devices | Saves ── */}
      <div className="gd-content">

        {/* Left column: Server Files + Installed On */}
        <div className="gd-main">

          {/* Server Files */}
          <section className="gd-section">
            <div className="gd-section-head">
              <h3 className="gd-section-title">
                <FontAwesomeIcon icon={faDatabase} /> Server Files
              </h3>
              <button className="gd-icon-btn" onClick={loadServerFiles} disabled={serverFilesLoading} title="Refresh">↺</button>
            </div>
            {game.path && <div className="gd-path-hint">{game.path}</div>}
            {serverFilesLoading && <div className="gd-empty">Loading...</div>}
            {!serverFilesLoading && serverFiles.length === 0 && (
              <div className="gd-empty">
                {game.path ? 'No files found at game path.' : 'No path set for this game.'}
              </div>
            )}
            {!serverFilesLoading && serverFiles.length > 0 && (
              <div className="gd-tree">
                {buildFileTree(serverFiles).map((node, i) => (
                  <FileTreeRow
                    key={node.isDir ? `d:${node.name}:${i}` : node.relativePath}
                    node={node}
                    depth={0}
                    deletingFile={deletingFile}
                    onDelete={handleDeleteServerFile}
                  />
                ))}
              </div>
            )}
          </section>

          {/* Steam Manifest */}
          <section className="gd-section">
            <div className="gd-section-head">
              <h3 className="gd-section-title">Steam Manifest</h3>
            </div>
            {!game.steamId ? (
              <div className="gd-empty">No Steam ID set — add one in Edit.</div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
                  <span style={{ color: '#a6adc8', fontSize: '0.85rem' }}>App ID: {game.steamId}</span>
                  {manifestInfo ? (
                    <span style={{ padding: '2px 8px', background: 'rgba(166,227,161,0.15)', border: '1px solid rgba(166,227,161,0.3)', borderRadius: '4px', color: '#a6e3a1', fontSize: '0.8rem' }}>
                      {manifestInfo.depots.length} depot{manifestInfo.depots.length !== 1 ? 's' : ''} ready
                    </span>
                  ) : (
                    <span style={{ padding: '2px 8px', background: 'rgba(166,173,200,0.1)', border: '1px solid rgba(166,173,200,0.2)', borderRadius: '4px', color: '#6c7086', fontSize: '0.8rem' }}>
                      No manifest
                    </span>
                  )}
                </div>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', alignItems: 'center' }}>
                  <button
                    className="gd-icon-btn"
                    onClick={handleFetchManifest}
                    disabled={manifestFetching}
                    title="Fetch manifest from Morrenus"
                    style={{ padding: '4px 10px', fontSize: '0.82rem' }}
                  >
                    {manifestFetching ? '…' : '⬇ Fetch from Morrenus'}
                  </button>
                  <button
                    className="gd-icon-btn"
                    onClick={() => manifestUploadRef.current?.click()}
                    title="Upload steamtoolz ZIP manually"
                    style={{ padding: '4px 10px', fontSize: '0.82rem' }}
                  >
                    ⬆ Upload ZIP
                  </button>
                  <input
                    ref={manifestUploadRef}
                    type="file"
                    accept=".zip"
                    style={{ display: 'none' }}
                    onChange={e => { if (e.target.files?.[0]) handleManifestUpload(e.target.files[0]); e.target.value = ''; }}
                  />
                </div>
                {manifestInfo && manifestInfo.depots.length > 0 && (
                  <div style={{ fontSize: '0.8rem', color: '#6c7086' }}>
                    Depots: {manifestInfo.depots.map(d => d.depotId).join(', ')}
                  </div>
                )}
                {manifestInfo && manifestInfo.depots.length > 0 && (
                  <div style={{ marginTop: '4px' }}>
                    <button
                      className="gd-icon-btn"
                      onClick={handleSteamDownload}
                      disabled={steamDownloading}
                      style={{ padding: '4px 10px', fontSize: '0.82rem' }}
                    >
                      {steamDownloading ? '…' : '⬇ Steam Download'}
                    </button>
                  </div>
                )}
              </div>
            )}
          </section>

          {/* Installed On */}
          <section className="gd-section">
            <div className="gd-section-head">
              <h3 className="gd-section-title">
                <FontAwesomeIcon icon={faMicrochip} /> Installed On
              </h3>
            </div>

            {(() => {
              const installedAgents = agents.filter(a => a.installedGames?.some(g => agentTitleMatches(g.title, game.title)));
              if (installedAgents.length === 0) {
                return <div className="gd-empty">Not installed on any registered device.</div>;
              }
              return installedAgents.map(agent => {
                const ig = agent.installedGames!.find(g => agentTitleMatches(g.title, game.title))!;
                const key = `${agent.id}:${ig.installPath}`;
                return (
                  <div key={agent.id} className="gd-device-card">
                    <div className="gd-device-header">
                      <span className={`gd-device-dot gd-dot-${agent.status}`} />
                      <span className="gd-device-name">{agent.name}</span>
                      <span className="gd-device-platform">{agent.platform}</span>
                      {ig.version && <span className="gd-tag">v{ig.version}</span>}
                      {ig.hasShortcut && <span className="gd-tag gd-tag-steam">Steam</span>}
                      {(ig.exePath || ig.scriptPath) && <span className="gd-tag gd-tag-installed">installed</span>}
                      <button
                        className="gd-launch-config-toggle"
                        title="Launch configuration"
                        onClick={() => setExpandedLaunchConfig(prev => ({ ...prev, [agent.id]: !prev[agent.id] }))}
                      >⚙ {expandedLaunchConfig[agent.id] ? '▲' : '▼'}</button>
                    </div>
                    <div className="gd-device-meta">
                      <span className="gd-device-path" title={ig.installPath}>{ig.installPath}</span>
                      <span className="gd-device-size">{formatBytes(ig.sizeBytes)}</span>
                    </div>
                    {ig.exeCandidates && ig.exeCandidates.length > 1 && (
                      <div className="gd-device-exe">
                        <span className="gd-device-exe-label">Launch exe:</span>
                        <select
                          className="gd-exe-select"
                          value={ig.exePath ?? ''}
                          disabled={agent.status !== 'online'}
                          onChange={e => handleChangeExe(agent.id, ig.title, e.target.value)}
                        >
                          {ig.exeCandidates.map(p => (
                            <option key={p} value={p}>{p.split('/').pop() || p.split('\\').pop() || p}</option>
                          ))}
                        </select>
                      </div>
                    )}
                    {expandedLaunchConfig[agent.id] && (
                      <div className="gd-device-launch-config">
                        <div className="gd-device-launch-field">
                          <label className="gd-device-launch-label">Args</label>
                          <input
                            type="text"
                            className="gd-device-launch-input"
                            value={agentLaunchArgs[agent.id] ?? ''}
                            placeholder="e.g. --rendering-driver vulkan"
                            onChange={e => setAgentLaunchArgs(prev => ({ ...prev, [agent.id]: e.target.value }))}
                          />
                        </div>
                        <div className="gd-device-launch-field">
                          <label className="gd-device-launch-label">Env Vars</label>
                          <textarea
                            className="gd-device-launch-textarea"
                            value={agentEnvVars[agent.id] ?? ''}
                            placeholder={'WINEDLLOVERRIDES=steam_api64=n,b\nDXVK_HUD=fps'}
                            rows={2}
                            onChange={e => setAgentEnvVars(prev => ({ ...prev, [agent.id]: e.target.value }))}
                          />
                        </div>
                        {(agentProtonVersions[agent.id] ?? []).length > 0 && (
                          <div className="gd-device-launch-field">
                            <label className="gd-device-launch-label">Proton</label>
                            <select
                              className="gd-exe-select gd-device-launch-select"
                              value={agentProtonPath[agent.id] ?? ''}
                              onChange={e => setAgentProtonPath(prev => ({ ...prev, [agent.id]: e.target.value }))}
                            >
                              <option value="">(auto)</option>
                              {(agentProtonVersions[agent.id] || []).map((v: ProtonVersionInfo) => (
                                <option key={v.binPath} value={v.binPath}>{v.name}</option>
                              ))}
                            </select>
                          </div>
                        )}
                        <div className="gd-device-launch-save">
                          <button
                            className="gd-icon-btn"
                            disabled={!!savingLaunchSettings[agent.id]}
                            onClick={() => handleSaveLaunchSettings(agent.id)}
                          >{savingLaunchSettings[agent.id] ? '⟳' : 'Save'}</button>
                        </div>
                      </div>
                    )}
                    <div className="gd-device-actions">
                      {agent.status === 'online' && (
                        <>
                          <button className="gd-icon-btn" title="Restart Steam on device" onClick={() => handleRestartSteam(agent.id)}>⟳ Steam</button>
                          <button className="gd-icon-btn" title="Reload run script" onClick={() => handleReloadScript(agent.id)}>⟳ Script</button>
                          {ig.scriptPath && (
                            <button
                              className="gd-icon-btn"
                              title="View run script"
                              disabled={!!requestingScript[ig.title]}
                              onClick={() => handleViewScript(agent.id, ig.title)}
                            >{requestingScript[ig.title] ? '⟳' : 'Script'}</button>
                          )}
                          <button className="gd-icon-btn" title="Reload Steam shortcut" onClick={() => handleReloadShortcut(agent.id)}>⟳ Shortcut</button>
                          <button className="gd-icon-btn" title="Refresh game list on device" onClick={() => handleRefreshAgent(agent.id)}>↺ Refresh</button>
                        </>
                      )}
                      {agent.status === 'online' && (ig.scriptPath || ig.exePath) && (
                        <button
                          className="gd-icon-btn"
                          title="View launch log"
                          disabled={!!requestingLog[ig.title]}
                          onClick={() => handleViewLog(agent.id, ig.title)}
                        >{requestingLog[ig.title] ? '⟳' : 'Log'}</button>
                      )}
                      {agent.status === 'online' && ig.scriptPath && (
                        <button
                          className="gd-icon-btn"
                          title="Rename Wine prefix"
                          onClick={() => handleRenamePrefix(agent.id)}
                        >Rename Prefix</button>
                      )}
                      <button
                        className="gd-icon-btn gd-icon-btn-danger"
                        title="Delete from device"
                        disabled={!!deletingFromAgent[key]}
                        onClick={() => handleDeleteFromAgent(agent.id, ig)}
                      >{deletingFromAgent[key] ? '⟳' : '✕ Remove'}</button>
                    </div>
                  </div>
                );
              });
            })()}
          </section>
        </div>

        {/* Right column: Saves */}
        <div className="gd-sidebar">

          {/* Save conflict banner */}
          {saveConflict && saveConflict.gameId === game.id && (
            <div className="save-conflict-banner">
              <div className="save-conflict-icon">⚠</div>
              <div className="save-conflict-body">
                <strong>Save conflict detected</strong>
                <p>
                  <em>{agents.find(a => a.id === saveConflict.uploadingAgentId)?.name ?? saveConflict.uploadingAgentId.slice(0, 8)}</em> uploaded saves,
                  but <em>{agents.find(a => a.id === saveConflict.conflictingAgentId)?.name ?? saveConflict.conflictingAgentId.slice(0, 8)}</em> had the previous latest.
                  Which should be kept?
                </p>
              </div>
              <div className="save-conflict-actions">
                <button className="saves-restore-btn" onClick={() => handleKeepSaves(saveConflict.uploadingAgentId, saveConflict.conflictingAgentId)}>
                  Keep {agents.find(a => a.id === saveConflict.uploadingAgentId)?.name ?? 'uploaded'}
                </button>
                <button className="saves-restore-btn" onClick={() => handleKeepSaves(saveConflict.conflictingAgentId, saveConflict.uploadingAgentId)}>
                  Keep {agents.find(a => a.id === saveConflict.conflictingAgentId)?.name ?? 'previous'}
                </button>
                <button className="saves-path-remove" onClick={() => setSaveConflict(null)}>✕</button>
              </div>
            </div>
          )}

          {/* Save Locations */}
          <section className="gd-section">
            <div className="gd-section-head">
              <h3 className="gd-section-title">
                <FontAwesomeIcon icon={faFolderOpen} /> Save Locations
              </h3>
              {savePathsInfo && savePathsInfo.source !== 'none' && (
                <span className={`saves-source-badge saves-source-${savePathsInfo.source}`}>
                  {savePathsInfo.source === 'manifest' ? 'Ludusavi'
                    : savePathsInfo.source === 'prefix-scan' ? 'Wine Prefix'
                    : 'Fallback'}
                </span>
              )}
            </div>

            {savePathsInfo && (() => {
              const customSet = new Set(savePathsInfo.customPaths);
              const detected = savePathsInfo.paths.filter(p => !customSet.has(p));
              return detected.length > 0 ? (
                <>
                  <div className="saves-section-label">Detected paths</div>
                  <ul className="saves-paths-list">
                    {detected.map(p => <li key={p} className="saves-path-item">{p}</li>)}
                  </ul>
                </>
              ) : null;
            })()}

            {savePathsInfo && Object.keys(savePathsInfo.agentPaths).length > 0 && (
              <>
                <div className="saves-section-label">Per-device custom paths</div>
                <ul className="saves-paths-list">
                  {Object.entries(savePathsInfo.agentPaths).map(([agentId, paths]) => {
                    const agentInfo = agents.find(a => a.id === agentId);
                    const agentName = agentInfo?.name ?? agentId.slice(0, 8);
                    return (
                      <li key={agentId} className="saves-path-item saves-path-item-custom">
                        <span className="saves-path-agent">{agentName}:</span>
                        <span className="saves-path-value">
                          {paths.map((p, i) => (
                            <span key={i} className="saves-path-entry">
                              {p}
                              {p.includes('*') && <span className="saves-glob-badge" title="Watches all matching subdirectories">glob</span>}
                            </span>
                          ))}
                        </span>
                        <button className="saves-path-edit" title="Edit paths" onClick={() => setBrowserAgent({ id: agentId, name: agentName, initialPath: paths[0] })}>✏</button>
                        <button className="saves-path-remove" title="Remove all" onClick={() => handleRemoveAgentPath(agentId)}>✕</button>
                      </li>
                    );
                  })}
                </ul>
              </>
            )}

            {savePathsInfo && savePathsInfo.paths.length === 0 && Object.keys(savePathsInfo.agentPaths).length === 0 && (
              <div className="saves-empty">No save paths detected.</div>
            )}

            {agents.filter(a => a.status === 'online').length > 0 && (
              <div className="saves-browse-row">
                {agents.filter(a => a.status === 'online').map(a => (
                  <button key={a.id} className="saves-browse-btn"
                    onClick={() => setBrowserAgent({ id: a.id, name: a.name })}>
                    <FontAwesomeIcon icon={faFolderOpen} /> Browse {a.name}
                  </button>
                ))}
              </div>
            )}
          </section>

          {/* Sync saves */}
          {(() => {
            const agentsWithGame = agents.filter(a =>
              a.status === 'online' && a.installedGames?.some(g => agentTitleMatches(g.title, game.title))
            );
            if (agentsWithGame.length === 0) return null;
            return (
              <section className="gd-section">
                <div className="gd-section-head">
                  <h3 className="gd-section-title">Sync Saves</h3>
                </div>
                <table className="gd-sync-table">
                  <thead>
                    <tr>
                      <th>Device</th>
                      <th>Upload</th>
                      <th>Restore</th>
                    </tr>
                  </thead>
                  <tbody>
                    {agentsWithGame.map(a => (
                      <tr key={a.id}>
                        <td>{a.name}</td>
                        <td><button className="saves-restore-btn" onClick={() => handleUploadFromAgent(a.id)}>↑ Upload</button></td>
                        <td>
                          <button
                            className="saves-restore-btn"
                            disabled={!!restoringOnAgent[a.id] || saveSnapshots.length === 0}
                            onClick={() => handleRestoreOnAgent(a.id)}
                            title={saveSnapshots.length === 0 ? 'No snapshots yet' : `Restore latest to ${a.name}`}
                          >{restoringOnAgent[a.id] ? '...' : '↓ Restore'}</button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>
            );
          })()}

          {/* Save Snapshots */}
          <section className="gd-section">
            <div className="gd-section-head">
              <h3 className="gd-section-title">Save Snapshots</h3>
              <button className="gd-icon-btn" onClick={() => { loadSaveSnapshots(); loadSavePathsInfo(); }} disabled={savesLoading} title="Refresh">↺</button>
            </div>
            {savesLoading && <div className="saves-empty">Loading...</div>}
            {!savesLoading && saveSnapshots.length === 0 && (
              <div className="saves-empty">No save backups yet. The agent uploads saves automatically when games exit.</div>
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
                      <button className="saves-delete-btn" title="Delete snapshot" onClick={() => handleDeleteSnapshot(snap.id)}>🗑</button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>
        </div>
      </div>

      {/* ══ SEARCH MODAL ══ */}
      {showSearchModal && (
        <div className="modal-overlay" onClick={() => setShowSearchModal(false)}>
          <div className="gd-search-modal" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Search Downloads — {game.title}</h3>
              <button className="modal-close" onClick={() => setShowSearchModal(false)}>×</button>
            </div>
            <div className="gd-search-modal-body">
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
                  <FontAwesomeIcon icon={searching ? faSpinner : faSearch} spin={searching} />
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
                  No results found. Check your indexer settings or try a different search term.
                </p>
              )}

              {error && <p className="error">{error}</p>}

              {results.length > 0 && (
                <div className="results-container">
                  <div className="results-header">
                    <h4>{t('searchResults')} ({results.length} {t('resultsFound')})</h4>
                  </div>
                  <div className="results-table">
                    <div className="results-header-row">
                      <div className="col-protocol sortable" onClick={() => handleSort('protocol')}>{t('protocol')} {getSortIcon('protocol')}</div>
                      <div className="col-score sortable" onClick={() => handleSort('scorePct')}>Score {getSortIcon('scorePct')}</div>
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
                      let explicitPlatform = '';
                      let explicitPlatformType: PlatformType | null = null;
                      if (result.category) {
                        const catIds = result.category.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));
                        for (const cid of catIds) {
                          const info = GetPlatformInfo(cid);
                          if (info.name !== 'Console' && info.name !== 'Unknown') {
                            explicitPlatform = info.name;
                            explicitPlatformType = info.type;
                            break;
                          }
                          if (!explicitPlatform && info.name !== 'Unknown') {
                            explicitPlatform = info.name;
                            explicitPlatformType = info.type;
                          }
                        }
                      }
                      const displayPlatform = explicitPlatform || analysis.detectedPlatform;
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
                          <div className="col-score">
                            {result.scorePct !== undefined ? (() => {
                              const bd = scoreBreakdown(result);
                              return (
                                <div className="score-cell">
                                  <div className="score-bar-track">
                                    <div className="score-bar-fill" style={{ width: `${result.scorePct}%`, background: scoreColor(result.scorePct) }} />
                                  </div>
                                  <span className="score-pct" style={{ color: scoreColor(result.scorePct) }}>{result.scorePct}%</span>
                                  <div className="score-tooltip">
                                    <div className="score-tooltip-row"><span>Title match</span><span>{result.titleScore ?? 0}% <span className="score-tooltip-pts">+{bd.titlePts}</span></span></div>
                                    <div className="score-tooltip-row"><span>Source{result.sourceName ? ` (${bd.sourceDesc})` : ''}</span><span>{result.sourceName ?? 'unknown'} <span className="score-tooltip-pts">+{bd.sourcePts}</span></span></div>
                                    <div className="score-tooltip-row"><span>Seeders</span><span>{result.protocol === 'nzb' ? 'NZB' : (result.seeders ?? 0)} <span className="score-tooltip-pts">+{bd.seederPts}</span></span></div>
                                    <div className="score-tooltip-row"><span>Size</span><span>{result.sizeWarning === 'too_large' ? 'too large' : result.sizeOutlier ? 'outlier' : 'ok'} <span className="score-tooltip-pts">+{bd.sizePts}</span></span></div>
                                    <div className="score-tooltip-total"><span>Total</span><span style={{ color: scoreColor(result.scorePct) }}>{result.scorePct} / 100</span></div>
                                  </div>
                                </div>
                              );
                            })() : <span className="score-pct">—</span>}
                          </div>
                          <div className="col-title">
                            <div className="title-content">
                              {result.infoUrl ? (
                                <a href={result.infoUrl} target="_blank" rel="noopener noreferrer" className="title-link">{result.title}</a>
                              ) : (
                                <span className="title-text">{result.title}</span>
                              )}
                              <div className="title-meta">
                                {result.sourceName && <span className={`title-tag source-badge source-${result.sourceName.toLowerCase().replace(/[^a-z0-9]/g, '')}`}>{result.sourceName}</span>}
                                {result.detectedVersion && <span className="title-tag version-badge">{result.detectedVersion}</span>}
                                {result.installMethod === 'RunInstaller' && <span className="title-tag installer-warning" title="Requires running an installer">Installer</span>}
                                {result.releaseType && result.releaseType !== 'game' && <span className={`title-tag reltype-${result.releaseType}`}>{result.releaseType.toUpperCase()}</span>}
                                {result.sizeWarning === 'too_large' && <span className="title-tag size-warning" title="Unusually large">⚠ Large</span>}
                                {result.sizeOutlier && <span className="title-tag size-warning" title="Much smaller than most results">⚠ Outlier</span>}
                                {result.releaseGroup && <span className="release-group">{result.releaseGroup}</span>}
                                {analysis.tags.map((tag, i) => <span key={i} className={`title-tag ${tag.toLowerCase()}`}>[{tag}]</span>)}
                              </div>
                            </div>
                          </div>
                          <div className="col-indexer"><span className="indexer-name">{result.indexer || result.indexerName}</span></div>
                          <div className="col-platform">
                            <span className="platform-tag" style={{ backgroundColor: finalColor }} title={`Category IDs: ${result.category || 'None'}`}>{displayPlatform}</span>
                          </div>
                          <div className="col-size"><span className="size">{result.formattedSize || `${(result.size / (1024 * 1024 * 1024)).toFixed(2)} GB`}</span></div>
                          <div className="col-peers">
                            {result.protocol?.toLowerCase() === 'usenet' || result.protocol?.toLowerCase() === 'nzb' ? (
                              <span className="peers-info">-</span>
                            ) : (
                              <div className="peers-info">
                                <span className={`seeders ${getSeedersClass(result.seeders)}`}><FontAwesomeIcon icon={faArrowUp} /> {result.seeders ?? 0}</span>
                                <span className="separator">/</span>
                                <span className="leechers"><FontAwesomeIcon icon={faArrowDown} /> {result.leechers ?? 0}</span>
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
                              {(() => {
                                const hash = result.magnetUrl ? extractInfoHash(result.magnetUrl) : '';
                                const isQueued = hash ? queuedInfoHashes.has(hash) : false;
                                return (<>
                                  {result.magnetUrl && (
                                    <button
                                      className={`download-btn magnet ${isQueued ? 'queued' : ''} ${downloadingUrl === result.magnetUrl ? 'loading' : ''}`}
                                      title={isQueued ? 'Already in download queue' : 'Send to Download Client'}
                                      onClick={() => handleDownload(result.magnetUrl, result.protocol)}
                                      disabled={!!downloadingUrl || isQueued}
                                    >{downloadingUrl === result.magnetUrl ? <FontAwesomeIcon icon={faSpinner} spin /> : isQueued ? <FontAwesomeIcon icon={faCheck} /> : <FontAwesomeIcon icon={faMagnet} />}</button>
                                  )}
                                  {result.downloadUrl && (
                                    <button
                                      className={`download-btn direct ${isQueued ? 'queued' : ''} ${downloadingUrl === result.downloadUrl ? 'loading' : ''}`}
                                      title={isQueued ? 'Already in download queue' : 'Send to Download Client'}
                                      onClick={() => handleDownload(result.downloadUrl, result.protocol)}
                                      disabled={!!downloadingUrl || isQueued}
                                    >{downloadingUrl === result.downloadUrl ? <FontAwesomeIcon icon={faSpinner} spin /> : isQueued ? <FontAwesomeIcon icon={faCheck} /> : <FontAwesomeIcon icon={faDownload} />}</button>
                                  )}
                                </>);
                              })()}
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Existing modals */}
      {game && (
        <UninstallModal
          isOpen={showUninstallModal}
          onClose={() => setShowUninstallModal(false)}
          onDelete={handleDeleteGame}
          gameTitle={game.title}
          gamePath={game.path}
          downloadPath={game.downloadPath}
        />
      )}

      {game && game.path && (
        <SwitchInstallerModal
          isOpen={showSwitchModal}
          onClose={() => setShowSwitchModal(false)}
          filePath={game.path}
          fileName={game.title}
        />
      )}

      {showCorrectionModal && game && (
        <GameCorrectionModal
          game={game}
          onClose={() => setShowCorrectionModal(false)}
          onSave={handleCorrectionSave}
        />
      )}

      {browserAgent && (
        <AgentFileBrowserModal
          agentId={browserAgent.id}
          agentName={browserAgent.name}
          initialPath={browserAgent.initialPath}
          onConfirm={(paths) => handleBrowseConfirm(browserAgent.id, paths)}
          onClose={() => setBrowserAgent(null)}
        />
      )}

      {/* Log modal */}
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
                background: '#11111b', color: '#cdd6f4', padding: '1rem', borderRadius: '6px',
                fontSize: '0.72rem', lineHeight: '1.5', overflowY: 'auto', maxHeight: '60vh',
                whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontFamily: 'monospace',
              }}>{logModal.content}</pre>
            </div>
          </div>
        </div>
      )}

      {scriptModal && (
        <div className="modal-overlay" onClick={() => setScriptModal(null)}>
          <div className="modal" style={{ maxWidth: '780px', width: '90vw' }} onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Run Script — {scriptModal.gameTitle}</h3>
              <button className="modal-close" onClick={() => setScriptModal(null)}>×</button>
            </div>
            <div className="modal-content">
              <div style={{ fontSize: '0.75rem', color: '#a6adc8', marginBottom: '0.5rem' }}>
                {scriptModal.agentName} · ~/Games/{scriptModal.gameTitle}/run.sh
              </div>
              <pre style={{
                background: '#11111b', color: '#cdd6f4', padding: '1rem', borderRadius: '6px',
                fontSize: '0.72rem', lineHeight: '1.5', overflowY: 'auto', maxHeight: '60vh',
                whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontFamily: 'monospace',
              }}>{scriptModal.content}</pre>
            </div>
          </div>
        </div>
      )}

      {/* Exe picker modal */}
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
                    <span className={`exe-type-badge ${c.type}`}>{c.type === 'installer' ? 'Installer' : 'Game'}</span>
                    <div className="exe-info">
                      <span className="exe-name">{c.name}</span>
                      <span className="exe-path">{c.fromArchive ? `inside ${c.fromArchive} · ` : ''}{c.relPath}</span>
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
              >Install</button>
            </div>
          </div>
        </div>
      )}

    </div>
  );
};

export default GameDetails;
