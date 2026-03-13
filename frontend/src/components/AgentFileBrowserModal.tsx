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

interface TreeNode {
  path: string;
  name: string;
  children: TreeNode[] | null;
  files: string[] | null; // null = not yet loaded
  expanded: boolean;
}

interface Props {
  agentId: string;
  agentName: string;
  initialPath?: string;
  onConfirm: (paths: string[]) => void;
  onClose: () => void;
}

function pathSep(p: string): string {
  return p.includes('\\') ? '\\' : '/';
}

function parentDir(p: string, s: string): string {
  const parts = p.split(s).filter(Boolean);
  if (!parts.length) return s;
  parts.pop();
  return (s + parts.join(s)) || s;
}

// Replace the segment at nodePath with * in currentPath.
// If currentPath starts with nodePath, preserves everything after it.
// Otherwise falls back to parent/* with no suffix.
function applyGlob(currentPath: string, nodePath: string, s: string): string {
  const parent = parentDir(nodePath, s);
  if (currentPath.startsWith(nodePath)) {
    const suffix = currentPath.slice(nodePath.length); // e.g. "/saves" or ""
    return parent + s + '*' + suffix;
  }
  return parent + s + '*';
}

function updateNodeByPath(
  node: TreeNode,
  targetPath: string,
  fn: (n: TreeNode) => TreeNode,
): TreeNode {
  if (node.path === targetPath) return fn(node);
  if (!node.children) return node;
  let changed = false;
  const newChildren = node.children.map(c => {
    const updated = updateNodeByPath(c, targetPath, fn);
    if (updated !== c) changed = true;
    return updated;
  });
  return changed ? { ...node, children: newChildren } : node;
}

const AgentFileBrowserModal: React.FC<Props> = ({ agentId, agentName, initialPath, onConfirm, onClose }) => {
  const [root, setRoot] = useState<TreeNode | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanding, setExpanding] = useState<string | null>(null);

  // currentPath: the path shown in the path bar (updated by clicks, used for glob context)
  const [currentPath, setCurrentPath] = useState<string>(initialPath ?? '');
  // pendingPaths: the accumulated list to confirm
  const [pendingPaths, setPendingPaths] = useState<string[]>(
    initialPath ? [initialPath] : []
  );

  const browse = useCallback(async (path: string): Promise<{ resolvedPath: string; dirs: TreeNode[]; files: string[] }> => {
    const encoded = encodeURIComponent(path);
    const res = await axios.get<BrowseResult>(`/api/v3/agent/${agentId}/browse?path=${encoded}`);
    const sorted = (res.data.entries ?? []).sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    const dirs = sorted
      .filter(e => e.isDir)
      .map(e => ({ path: e.path, name: e.name, children: null, files: null, expanded: false }));
    const files = sorted.filter(e => !e.isDir).map(e => e.name);
    return { resolvedPath: res.data.path, dirs, files };
  }, [agentId]);

  useEffect(() => {
    let cancelled = false;

    async function init() {
      try {
        setLoading(true);
        setError(null);

        const { resolvedPath: homePath } = await browse('~');
        if (cancelled) return;

        const s = pathSep(homePath);
        const rootPath = s === '\\' ? homePath.substring(0, homePath.indexOf(s) + 1) : '/';

        let expandTarget = homePath;
        if (initialPath) {
          if (initialPath.includes('*')) {
            const parts = initialPath.split(s);
            const wildIdx = parts.findIndex(p => p.includes('*'));
            const expandParts = parts.slice(0, wildIdx).filter(Boolean);
            expandTarget = expandParts.length ? s + expandParts.join(s) : rootPath;
          } else {
            expandTarget = initialPath;
          }
        }

        const toExpand: string[] = [rootPath];
        if (expandTarget !== rootPath) {
          const parts = expandTarget.replace(/^[/\\]+/, '').split(s).filter(Boolean);
          let acc = s === '\\' ? rootPath.slice(0, -1) : '';
          for (const part of parts) {
            acc += s + part;
            toExpand.push(acc);
          }
        }

        let tree: TreeNode = { path: rootPath, name: rootPath, children: null, files: null, expanded: false };
        for (const p of toExpand) {
          if (cancelled) return;
          try {
            const { dirs, files } = await browse(p);
            if (cancelled) return;
            tree = updateNodeByPath(tree, p, n => ({ ...n, children: dirs, files, expanded: true }));
          } catch { break; }
        }

        if (!cancelled) {
          setRoot(tree);
          setLoading(false);
        }
      } catch (err: any) {
        if (!cancelled) {
          setError(err.message || 'Failed to load');
          setLoading(false);
        }
      }
    }

    init();
    return () => { cancelled = true; };
  }, [agentId, initialPath, browse]);

  const handleToggle = useCallback(async (node: TreeNode) => {
    if (node.expanded) {
      setRoot(r => r ? updateNodeByPath(r, node.path, n => ({ ...n, expanded: false })) : null);
      return;
    }
    if (node.children !== null) {
      setRoot(r => r ? updateNodeByPath(r, node.path, n => ({ ...n, expanded: true })) : null);
      return;
    }
    setExpanding(node.path);
    try {
      const { dirs, files } = await browse(node.path);
      setRoot(r => r ? updateNodeByPath(r, node.path, n => ({ ...n, children: dirs, files, expanded: true })) : null);
    } catch {
      setRoot(r => r ? updateNodeByPath(r, node.path, n => ({ ...n, children: [], files: [], expanded: true })) : null);
    } finally {
      setExpanding(null);
    }
  }, [browse]);

  const addPath = (path: string) => {
    if (!path) return;
    setPendingPaths(prev => prev.includes(path) ? prev : [...prev, path]);
    setCurrentPath(path);
  };

  const removePath = (path: string) => {
    setPendingPaths(prev => prev.filter(p => p !== path));
  };

  const handleFolderClick = (node: TreeNode) => {
    setCurrentPath(node.path);
  };

  const handleGlobClick = (node: TreeNode) => {
    const s = pathSep(node.path);
    const glob = applyGlob(currentPath || node.path, node.path, s);
    addPath(glob);
  };

  const renderTree = (node: TreeNode, depth: number): React.ReactNode => {
    const isLoading = expanding === node.path;

    return (
      <div key={node.path} className="ftree-node-wrap">
        <div
          className={`ftree-row${currentPath === node.path ? ' ftree-row--active' : ''}`}
          style={{ paddingLeft: `${depth * 18 + 10}px` }}
        >
          <button className="ftree-toggle" onClick={() => handleToggle(node)} tabIndex={-1}>
            {isLoading ? <span className="ftree-spinner">⟳</span> : node.expanded ? '▾' : '▸'}
          </button>
          <span
            className="ftree-name"
            onClick={() => handleFolderClick(node)}
            title={`Set path: ${node.path}`}
          >
            <span className="ftree-icon">📁</span>
            {node.name === '/' ? '/ (root)' : node.name}
          </span>
          <button
            className="ftree-glob-btn"
            onClick={e => { e.stopPropagation(); handleGlobClick(node); }}
            title={`Wildcard this level`}
          >
            *
          </button>
        </div>
        {node.expanded && node.children !== null && (
          <div>
            {node.children.length === 0 && (!node.files || node.files.length === 0)
              ? <div className="ftree-empty" style={{ paddingLeft: `${(depth + 1) * 18 + 32}px` }}>Empty</div>
              : <>
                  {node.children.map(child => renderTree(child, depth + 1))}
                  {node.files?.map(f => (
                    <div
                      key={f}
                      className="ftree-file"
                      style={{ paddingLeft: `${(depth + 1) * 18 + 32}px` }}
                    >
                      {f}
                    </div>
                  ))}
                </>
            }
          </div>
        )}
      </div>
    );
  };

  const canConfirm = pendingPaths.length > 0;

  return (
    <div className="ftree-overlay" onClick={onClose}>
      <div className="ftree-modal" onClick={e => e.stopPropagation()}>
        <div className="ftree-header">
          <span className="ftree-title">Browse {agentName}</span>
          <button className="ftree-close" onClick={onClose}>✕</button>
        </div>
        <div className="ftree-hint">
          Click a folder to set the path bar &nbsp;·&nbsp; Click <span className="ftree-glob-badge">*</span> to add a wildcard at that level and add to list
        </div>
        <div className="ftree-body">
          {loading && <div className="ftree-status">Loading...</div>}
          {!loading && error && <div className="ftree-status ftree-status--error">{error}</div>}
          {!loading && !error && root && renderTree(root, 0)}
        </div>

        {/* Footer: path bar + pending list + actions */}
        <div className="ftree-footer">
          {/* Current path bar */}
          <div className="ftree-pathbar">
            <span className="ftree-pathbar-label">Path</span>
            <span className="ftree-pathbar-value">{currentPath || <span className="ftree-pathbar-empty">— click a folder —</span>}</span>
            <button
              className="ftree-add-btn"
              onClick={() => addPath(currentPath)}
              disabled={!currentPath}
              title="Add to list"
            >
              + Add
            </button>
          </div>

          {/* Pending list */}
          {pendingPaths.length > 0 && (
            <div className="ftree-pending">
              {pendingPaths.map(p => (
                <div key={p} className="ftree-pending-item">
                  <span className="ftree-pending-path">
                    {p}
                    {p.includes('*') && <span className="ftree-glob-tag">glob</span>}
                  </span>
                  <button className="ftree-pending-remove" onClick={() => removePath(p)} title="Remove">×</button>
                </div>
              ))}
            </div>
          )}

          {/* Action buttons */}
          <div className="ftree-actions">
            <button className="ftree-cancel-btn" onClick={onClose}>Cancel</button>
            <button
              className="ftree-confirm-btn"
              onClick={() => onConfirm(pendingPaths)}
              disabled={!canConfirm}
            >
              Confirm{pendingPaths.length > 1 ? ` (${pendingPaths.length})` : ''}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default AgentFileBrowserModal;
