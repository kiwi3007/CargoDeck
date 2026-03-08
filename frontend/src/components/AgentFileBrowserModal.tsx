import React, { useEffect, useState, useCallback } from 'react';
import axios from 'axios';

interface DirEntry {
  name: string;
  path: string;
  isDir: boolean;
}

interface BrowseResult {
  path: string;
  entries: DirEntry[];
  error?: string;
}

interface Props {
  agentId: string;
  agentName: string;
  initialPath?: string;
  onSelect: (path: string) => void;
  onClose: () => void;
}

function parentPath(p: string): string {
  const sep = p.includes('\\') ? '\\' : '/';
  const parts = p.split(sep).filter(Boolean);
  if (parts.length === 0) return sep;
  parts.pop();
  const result = sep + parts.join(sep);
  return result || sep;
}

function breadcrumbSegments(p: string): { label: string; path: string }[] {
  const sep = p.includes('\\') ? '\\' : '/';
  const parts = p.split(sep).filter(Boolean);
  const segs: { label: string; path: string }[] = [];
  let acc = '';
  for (const part of parts) {
    acc = acc + sep + part;
    segs.push({ label: part, path: acc });
  }
  return segs;
}

const AgentFileBrowserModal: React.FC<Props> = ({ agentId, agentName, initialPath, onSelect, onClose }) => {
  const [currentPath, setCurrentPath] = useState<string>('');
  const [entries, setEntries] = useState<DirEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const navigate = useCallback(async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      const encoded = encodeURIComponent(path || '~');
      const res = await axios.get<BrowseResult>(`/api/v3/agent/${agentId}/browse?path=${encoded}`);
      setCurrentPath(res.data.path);
      // Sort: dirs first, then files, each group alphabetically
      const sorted = [...(res.data.entries || [])].sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        return a.name.localeCompare(b.name);
      });
      setEntries(sorted);
    } catch (err: any) {
      setError(err.response?.data?.error || err.message || 'Failed to browse directory');
    } finally {
      setLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    navigate(initialPath || '~');
  }, [navigate, initialPath]);

  const segs = breadcrumbSegments(currentPath);
  const isRoot = segs.length <= 1;

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-container file-browser-modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <span className="modal-title">Browse {agentName}</span>
          <button className="modal-close" onClick={onClose}>✕</button>
        </div>

        {/* Breadcrumb */}
        <div className="file-browser-breadcrumb">
          <span className="breadcrumb-sep">/</span>
          {segs.map((seg, i) => (
            <React.Fragment key={seg.path}>
              <button
                className="breadcrumb-seg"
                onClick={() => navigate(seg.path)}
                disabled={i === segs.length - 1}
              >
                {seg.label}
              </button>
              {i < segs.length - 1 && <span className="breadcrumb-sep">/</span>}
            </React.Fragment>
          ))}
        </div>

        {/* Select current folder button */}
        {currentPath && (
          <div className="file-browser-select-row">
            <button className="file-browser-select-btn" onClick={() => onSelect(currentPath)}>
              Select this folder
            </button>
            <span className="file-browser-current-path">{currentPath}</span>
          </div>
        )}

        {/* Directory listing */}
        <div className="file-browser-list">
          {loading && <div className="file-browser-loading">Loading...</div>}
          {!loading && error && <div className="file-browser-error">{error}</div>}
          {!loading && !error && (
            <>
              {!isRoot && (
                <div
                  className="file-browser-entry file-browser-entry-dir"
                  onClick={() => navigate(parentPath(currentPath))}
                >
                  <span className="file-browser-icon">📁</span>
                  <span className="file-browser-name">..</span>
                </div>
              )}
              {entries.map(entry => (
                <div
                  key={entry.path}
                  className={`file-browser-entry ${entry.isDir ? 'file-browser-entry-dir' : 'file-browser-entry-file'}`}
                  onClick={() => entry.isDir && navigate(entry.path)}
                  title={entry.isDir ? `Open ${entry.name}` : entry.name}
                >
                  <span className="file-browser-icon">{entry.isDir ? '📁' : '📄'}</span>
                  <span className="file-browser-name">{entry.name}</span>
                  {entry.isDir && (
                    <button
                      className="file-browser-select-inline"
                      onClick={e => { e.stopPropagation(); onSelect(entry.path); }}
                      title={`Select ${entry.name}`}
                    >
                      ↩
                    </button>
                  )}
                </div>
              ))}
              {entries.length === 0 && <div className="file-browser-empty">Empty directory</div>}
            </>
          )}
        </div>
      </div>
    </div>
  );
};

export default AgentFileBrowserModal;
