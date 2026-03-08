import React, { useEffect, useState } from 'react';
import axios from 'axios';
import './Devices.css';

interface InstallPath {
  path: string;
  label: string;
  freeBytes: number;
}

interface ActiveJob {
  jobId: string;
  gameTitle: string;
  status: string;
  message: string;
  percent: number;
  updatedAt: string;
}

interface InstalledGame {
  title: string;
  installPath: string;
  exePath?: string;
  scriptPath?: string;
  sizeBytes: number;
  hasShortcut: boolean;
  version?: string;
}

interface AgentInfo {
  id: string;
  name: string;
  platform: string;
  steamPath?: string;
  status: 'online' | 'offline';
  lastSeen: string;
  installPaths?: InstallPath[];
  currentJob?: ActiveJob;
  recentJobs?: ActiveJob[];
  installedGames?: InstalledGame[];
  lastScanned?: string;
}

function formatBytes(bytes: number): string {
  if (bytes < 0) return 'Unknown';
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + '\u00a0' + sizes[i];
}

function formatTimeAgo(dateStr: string): string {
  if (!dateStr) return 'never';
  const date = new Date(dateStr);
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 30) return 'just now';
  if (seconds < 90) return '1 min ago';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} hr ago`;
  return `${Math.floor(hours / 24)} days ago`;
}

function jobStatusColor(status: string): string {
  switch (status) {
    case 'done':             return '#34d399';
    case 'failed':           return '#f87171';
    case 'downloading':      return '#38bdf8';
    case 'extracting':       return '#a78bfa';
    case 'installing':       return '#fb923c';
    case 'creating_shortcut':return '#22d3ee';
    default:                 return '#475569';
  }
}

function jobStatusLabel(status: string): string {
  switch (status) {
    case 'done':             return 'Done';
    case 'failed':           return 'Failed';
    case 'queued':           return 'Queued';
    case 'downloading':      return 'Downloading';
    case 'extracting':       return 'Extracting';
    case 'installing':       return 'Installing';
    case 'creating_shortcut':return 'Creating shortcut';
    default:                 return status;
  }
}

const Devices: React.FC = () => {
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [scanning, setScanning] = useState<Record<string, boolean>>({});
  const [refreshing, setRefreshing] = useState<Record<string, boolean>>({});
  const [regenning, setRegenning] = useState<Record<string, boolean>>({});
  const [restartingSteam, setRestartingSteam] = useState<Record<string, boolean>>({});
  const [deleting, setDeleting] = useState<Record<string, boolean>>({});
  // Titles (lowercase) of library games that have an update available
  const [gamesWithUpdates, setGamesWithUpdates] = useState<Set<string>>(new Set());

  useEffect(() => {
    // Load library games to know which titles have updates pending
    axios.get<Array<{ title: string; updateAvailable: boolean }>>('/api/v3/game')
      .then(r => {
        const titles = new Set<string>(
          (r.data || []).filter(g => g.updateAvailable).map(g => g.title.toLowerCase())
        );
        setGamesWithUpdates(titles);
      })
      .catch(() => { /* ignore */ });

    const updateHandler = (e: Event) => {
      try {
        const data = JSON.parse((e as CustomEvent).detail) as { title: string };
        setGamesWithUpdates(prev => new Set([...prev, data.title.toLowerCase()]));
      } catch { /* ignore */ }
    };
    window.addEventListener('GAME_UPDATE_AVAILABLE_EVENT', updateHandler);

    axios.get<AgentInfo[]>('/api/v3/agent')
      .then(r => { setAgents(r.data || []); setLoading(false); })
      .catch(() => setLoading(false));

    const handler = (e: Event) => {
      try {
        const list = JSON.parse((e as CustomEvent).detail) as AgentInfo[];
        setAgents(list || []);
        setLoading(false);
        setScanning(prev => {
          const next = { ...prev };
          for (const a of list) {
            if (a.installedGames !== undefined) delete next[a.id];
          }
          return next;
        });
      } catch { /* ignore */ }
    };
    window.addEventListener('AGENTS_UPDATED_EVENT', handler);
    return () => {
      window.removeEventListener('AGENTS_UPDATED_EVENT', handler);
      window.removeEventListener('GAME_UPDATE_AVAILABLE_EVENT', updateHandler);
    };
  }, []);

  const handleScan = async (agentId: string) => {
    setScanning(prev => ({ ...prev, [agentId]: true }));
    try {
      await axios.post(`/api/v3/agent/${agentId}/scan`);
    } catch {
      setScanning(prev => ({ ...prev, [agentId]: false }));
    }
  };

  const handleRestartSteam = async (agentId: string) => {
    setRestartingSteam(prev => ({ ...prev, [agentId]: true }));
    try {
      await axios.post(`/api/v3/agent/${agentId}/restart-steam`);
    } catch (err: any) {
      alert(`Steam restart failed: ${err.response?.data?.error || err.message}`);
    } finally {
      setTimeout(() => setRestartingSteam(prev => ({ ...prev, [agentId]: false })), 5000);
    }
  };

  const handleRegenScripts = async (agentId: string) => {
    setRegenning(prev => ({ ...prev, [agentId]: true }));
    try {
      await axios.post(`/api/v3/agent/${agentId}/regen-scripts`);
    } catch (err: any) {
      alert(`Script regeneration failed: ${err.response?.data?.error || err.message}`);
    } finally {
      setTimeout(() => setRegenning(prev => ({ ...prev, [agentId]: false })), 3000);
    }
  };

  const handleRefreshShortcuts = async (agentId: string) => {
    setRefreshing(prev => ({ ...prev, [agentId]: true }));
    try {
      await axios.post(`/api/v3/agent/${agentId}/refresh-shortcuts`);
    } catch (err: any) {
      alert(`Shortcut refresh failed: ${err.response?.data?.error || err.message}`);
    } finally {
      setTimeout(() => setRefreshing(prev => ({ ...prev, [agentId]: false })), 2000);
    }
  };

  const handleDelete = async (agentId: string, game: InstalledGame, removeShortcut: boolean) => {
    const key = `${agentId}:${game.installPath}`;
    if (!window.confirm(`Delete "${game.title}" from this device?${removeShortcut ? '\nSteam shortcut will also be removed.' : ''}`)) return;
    setDeleting(prev => ({ ...prev, [key]: true }));
    try {
      await axios.delete(`/api/v3/agent/${agentId}/game`, {
        data: { title: game.title, installPath: game.installPath, removeShortcut }
      });
    } catch (err: any) {
      alert(`Delete failed: ${err.response?.data?.error || err.message}`);
    } finally {
      setDeleting(prev => ({ ...prev, [key]: false }));
    }
  };

  const sorted = [...agents].sort((a, b) => {
    if (a.status === b.status) return a.name.localeCompare(b.name);
    return a.status === 'online' ? -1 : 1;
  });

  const onlineCount = agents.filter(a => a.status === 'online').length;

  return (
    <div className="devices-page">
      <header className="devices-header">
        <div>
          <div className="devices-header-eyebrow">PLAYERR · NETWORK</div>
          <h1 className="devices-header-title">DEVICES</h1>
        </div>
        {!loading && agents.length > 0 && (
          <div className="devices-header-stats">
            <div className="devices-stat">
              <span className="devices-stat-value">{onlineCount}</span>
              <span className="devices-stat-label">ONLINE</span>
            </div>
            <div className="devices-stat">
              <span className="devices-stat-value">{agents.length}</span>
              <span className="devices-stat-label">TOTAL</span>
            </div>
          </div>
        )}
      </header>

      {loading && (
        <div className="devices-empty">
          <p>Connecting...</p>
        </div>
      )}

      {!loading && sorted.length === 0 && (
        <div className="devices-empty">
          <div className="devices-empty-icon">📡</div>
          <p>No agents registered yet.</p>
          <p className="devices-empty-hint">
            Run the setup command from Settings → Agents on any device.
          </p>
        </div>
      )}

      <div className="devices-grid">
        {sorted.map((agent, idx) => (
          <div
            key={agent.id}
            className={`device-card ${agent.status}`}
            style={{ animationDelay: `${idx * 0.07}s` }}
          >
            {/* Gradient accent bar */}
            <div className="device-card-accent" />

            {/* Job progress stripe (thin overlay bar when job active) */}
            {agent.currentJob && (
              <div className="device-job-stripe-track">
                <div
                  className="device-job-stripe-fill"
                  style={{
                    width: `${agent.currentJob.percent}%`,
                    background: jobStatusColor(agent.currentJob.status),
                  }}
                />
              </div>
            )}

            <div className="device-card-body">
              {/* Identity */}
              <div className="device-ident">
                <div className="device-status-indicator">
                  <div className={`device-dot ${agent.status}`} />
                  {agent.status === 'online' && <div className="device-dot-ring" />}
                </div>
                <div className="device-ident-info">
                  <span className="device-name">{agent.name}</span>
                  <span className="device-platform">{agent.platform}</span>
                </div>
                <span className="device-lastseen">{formatTimeAgo(agent.lastSeen)}</span>
              </div>

              {/* Active job */}
              {agent.currentJob && (
                <div className="device-active-job">
                  <div className="device-job-top">
                    <span className="device-job-name">{agent.currentJob.gameTitle}</span>
                    <span className="device-job-pct" style={{ color: jobStatusColor(agent.currentJob.status) }}>
                      {agent.currentJob.percent}%
                    </span>
                  </div>
                  <div className="device-job-msg" style={{ color: jobStatusColor(agent.currentJob.status) }}>
                    {agent.currentJob.message || jobStatusLabel(agent.currentJob.status)}
                  </div>
                  <div className="device-bar-track">
                    <div
                      className="device-bar-fill"
                      style={{
                        width: `${agent.currentJob.percent}%`,
                        background: jobStatusColor(agent.currentJob.status),
                      }}
                    />
                  </div>
                </div>
              )}

              {/* Installed games */}
              <div className="device-section">
                <div className="device-section-head">
                  <span className="device-section-title">
                    INSTALLED
                    {agent.installedGames !== undefined && (
                      <span className="device-section-count">{agent.installedGames.length}</span>
                    )}
                  </span>
                  <div className="device-section-actions">
                    {agent.lastScanned && (
                      <span className="device-scan-time">scanned {formatTimeAgo(agent.lastScanned)}</span>
                    )}
                    {agent.status === 'online' && (
                      <>
                        <button
                          className="device-scan-btn"
                          onClick={() => handleRestartSteam(agent.id)}
                          disabled={restartingSteam[agent.id]}
                          title="Restart Steam"
                        >
                          {restartingSteam[agent.id] ? '⟳' : '⏻'} Restart Steam
                        </button>
                        <button
                          className="device-scan-btn"
                          onClick={() => handleRegenScripts(agent.id)}
                          disabled={regenning[agent.id]}
                          title="Regenerate launch scripts"
                        >
                          {regenning[agent.id] ? '⟳' : '✎'} Scripts
                        </button>
                        <button
                          className="device-scan-btn"
                          onClick={() => handleRefreshShortcuts(agent.id)}
                          disabled={refreshing[agent.id]}
                          title="Refresh Steam shortcuts"
                        >
                          {refreshing[agent.id] ? '⟳' : '⊞'} Shortcuts
                        </button>
                        <button
                          className="device-scan-btn"
                          onClick={() => handleScan(agent.id)}
                          disabled={scanning[agent.id]}
                          title="Scan installed games"
                        >
                          {scanning[agent.id] ? '⟳' : '↺'} Scan
                        </button>
                      </>
                    )}
                  </div>
                </div>

                {agent.installedGames === undefined && agent.status === 'online' && !scanning[agent.id] && (
                  <p className="device-games-empty">Click <strong>Scan</strong> to detect installed games on this device.</p>
                )}
                {scanning[agent.id] && (
                  <p className="device-games-empty">Scanning...</p>
                )}
                {agent.installedGames !== undefined && agent.installedGames.length === 0 && !scanning[agent.id] && (
                  <p className="device-games-empty">No games installed.</p>
                )}

                {agent.installedGames && agent.installedGames.length > 0 && !scanning[agent.id] && (
                  <div className="device-games-list">
                    {agent.installedGames.map((game, i) => {
                      const key = `${agent.id}:${game.installPath}`;
                      const isDeleting = deleting[key];
                      return (
                        <div key={i} className="device-game-row">
                          <div className="device-game-info">
                            <span className="device-game-title">{game.title}</span>
                            <div className="device-game-meta">
                              <span className="device-game-size">{formatBytes(game.sizeBytes)}</span>
                              {game.version && (
                                <span className="device-game-version">v{game.version}</span>
                              )}
                              {gamesWithUpdates.has(game.title.toLowerCase()) && (
                                <span className="device-game-badge update-available">update</span>
                              )}
                              {game.exePath || game.scriptPath ? (
                                <span className="device-game-badge installed">installed</span>
                              ) : (
                                <span className="device-game-badge files-only">files</span>
                              )}
                              {game.hasShortcut && (
                                <span className="device-game-badge shortcut">steam</span>
                              )}
                            </div>
                          </div>
                          <div className="device-game-actions">
                            {game.hasShortcut && (
                              <button
                                className="device-game-btn shortcut-remove"
                                title="Remove Steam shortcut"
                                disabled={isDeleting}
                                onClick={() => handleDelete(agent.id, game, true)}
                              >
                                ⊘
                              </button>
                            )}
                            <button
                              className="device-game-btn danger"
                              title="Delete game files"
                              disabled={isDeleting}
                              onClick={() => handleDelete(agent.id, game, game.hasShortcut)}
                            >
                              {isDeleting ? '⟳' : '✕'}
                            </button>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              {/* Storage */}
              {agent.installPaths && agent.installPaths.length > 0 && (
                <div className="device-section">
                  <div className="device-section-head">
                    <span className="device-section-title">STORAGE</span>
                  </div>
                  {agent.installPaths.map((p, i) => (
                    <div key={i} className="device-storage-row">
                      <span className="device-storage-label">{p.label}</span>
                      <span className="device-storage-free">
                        {p.freeBytes >= 0 ? formatBytes(p.freeBytes) + ' free' : '—'}
                      </span>
                    </div>
                  ))}
                </div>
              )}

              {/* Recent jobs */}
              {agent.recentJobs && agent.recentJobs.length > 0 && (
                <div className="device-section">
                  <div className="device-section-head">
                    <span className="device-section-title">RECENT</span>
                  </div>
                  {agent.recentJobs.slice(0, 5).map((job, i) => (
                    <div key={i} className="device-recent-row">
                      <span className="device-recent-icon" style={{ color: jobStatusColor(job.status) }}>
                        {job.status === 'done' ? '✓' : '✗'}
                      </span>
                      <span className="device-recent-game">{job.gameTitle}</span>
                      <span className="device-recent-time">{formatTimeAgo(job.updatedAt)}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export default Devices;
