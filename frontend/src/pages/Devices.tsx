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
    case 'done': return '#a6e3a1';
    case 'failed': return '#f38ba8';
    case 'downloading': return '#89b4fa';
    case 'extracting': return '#cba6f7';
    case 'installing': return '#fab387';
    case 'creating_shortcut': return '#94e2d5';
    default: return '#a6adc8';
  }
}

function jobStatusLabel(status: string): string {
  switch (status) {
    case 'done': return 'Done';
    case 'failed': return 'Failed';
    case 'queued': return 'Queued';
    case 'downloading': return 'Downloading';
    case 'extracting': return 'Extracting';
    case 'installing': return 'Installing';
    case 'creating_shortcut': return 'Creating shortcut';
    default: return status;
  }
}

const Devices: React.FC = () => {
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let alive = true;
    const fetchAgents = async () => {
      try {
        const res = await axios.get<AgentInfo[]>('/api/v3/agent');
        if (alive) setAgents(res.data || []);
      } catch {
        // ignore
      } finally {
        if (alive) setLoading(false);
      }
    };

    fetchAgents();
    const interval = setInterval(fetchAgents, 5000);
    const handler = () => fetchAgents();
    window.addEventListener('AGENT_PROGRESS_EVENT', handler);

    return () => {
      alive = false;
      clearInterval(interval);
      window.removeEventListener('AGENT_PROGRESS_EVENT', handler);
    };
  }, []);

  const sorted = [...agents].sort((a, b) => {
    if (a.status === b.status) return a.name.localeCompare(b.name);
    return a.status === 'online' ? -1 : 1;
  });

  return (
    <div className="devices-page">
      <div className="devices-header">
        <h1>Devices</h1>
        <p>Connected remote agents and their install status.</p>
      </div>

      {loading && <div className="devices-empty">Loading...</div>}

      {!loading && sorted.length === 0 && (
        <div className="devices-empty">
          <div className="devices-empty-icon">📡</div>
          <p>No agents registered yet.</p>
          <p className="devices-empty-hint">
            Download and run the Playerr agent on a device to connect it here.
          </p>
        </div>
      )}

      <div className="devices-grid">
        {sorted.map(agent => (
          <div key={agent.id} className={`device-card ${agent.status}`}>
            {/* Header */}
            <div className="device-card-header">
              <div className="device-card-title">
                <span className="device-name">{agent.name}</span>
                <span className={`device-status-badge ${agent.status}`}>
                  {agent.status === 'online' ? '● Online' : '○ Offline'}
                </span>
              </div>
              <div className="device-card-meta">
                <span className="device-platform">{agent.platform}</span>
                <span className="device-lastseen">Last seen {formatTimeAgo(agent.lastSeen)}</span>
              </div>
            </div>

            {/* Current job */}
            {agent.currentJob && (
              <div className="device-current-job">
                <div className="device-job-header">
                  <span className="device-job-title">{agent.currentJob.gameTitle}</span>
                  <span className="device-job-percent" style={{ color: jobStatusColor(agent.currentJob.status) }}>
                    {agent.currentJob.percent}%
                  </span>
                </div>
                <div className="device-job-status" style={{ color: jobStatusColor(agent.currentJob.status) }}>
                  {agent.currentJob.message || jobStatusLabel(agent.currentJob.status)}
                </div>
                <div className="device-progress-track">
                  <div
                    className="device-progress-fill"
                    style={{
                      width: `${agent.currentJob.percent}%`,
                      background: jobStatusColor(agent.currentJob.status),
                    }}
                  />
                </div>
              </div>
            )}

            {/* Install paths / storage */}
            {agent.installPaths && agent.installPaths.length > 0 && (
              <div className="device-section">
                <div className="device-section-label">Storage</div>
                {agent.installPaths.map((p, i) => (
                  <div key={i} className="device-path-row">
                    <span className="device-path-label">{p.label}</span>
                    <span className="device-path-space">
                      {p.freeBytes >= 0 ? formatBytes(p.freeBytes) + ' free' : '—'}
                    </span>
                  </div>
                ))}
              </div>
            )}

            {/* Recent jobs */}
            {agent.recentJobs && agent.recentJobs.length > 0 && (
              <div className="device-section">
                <div className="device-section-label">Recent</div>
                {agent.recentJobs.slice(0, 5).map((job, i) => (
                  <div key={i} className="device-recent-row">
                    <span className="device-recent-icon" style={{ color: jobStatusColor(job.status) }}>
                      {job.status === 'done' ? '✓' : '✗'}
                    </span>
                    <span className="device-recent-title">{job.gameTitle}</span>
                    <span className="device-recent-time">{formatTimeAgo(job.updatedAt)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
};

export default Devices;
