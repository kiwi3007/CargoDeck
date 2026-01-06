import React, { useEffect, useState } from 'react';
import { useTranslation } from '../i18n/translations';
import './Status.css';

interface DownloadStatus {
    clientId: number;
    id: string;
    name: string;
    size: number;
    progress: number;
    state: number; // Enum: 0=Downloading, 1=Paused, 2=Completed, 3=Error, 4=Queued, 5=Checking, 6=Deleted, 7=Importing
    category: string;
    downloadPath: string;
}

const Status: React.FC = () => {
    const { t } = useTranslation();
    const [downloads, setDownloads] = useState<DownloadStatus[]>([]);
    const [loading, setLoading] = useState(true);

    const fetchQueue = async () => {
        try {
            const response = await fetch('http://127.0.0.1:5002/api/v3/downloadclient/queue');
            if (response.ok) {
                const data = await response.json();
                setDownloads(data);
            }
        } catch (error) {
            console.error('Error fetching queue:', error);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        fetchQueue();
        const interval = setInterval(fetchQueue, 3000);
        return () => clearInterval(interval);
    }, []);

    const handlePause = async (clientId: number, downloadId: string, e: React.MouseEvent) => {
        e.stopPropagation();
        try {
            await fetch(`http://127.0.0.1:5002/api/v3/downloadclient/queue/${clientId}/${encodeURIComponent(downloadId)}/pause`, {
                method: 'POST'
            });
            // Optimistic update
            setDownloads(prev => prev.map(d =>
                d.id === downloadId ? { ...d, state: 1 } : d
            ));
        } catch (error) {
            console.error('Error pausing download:', error);
        }
    };

    const handleResume = async (clientId: number, downloadId: string, e: React.MouseEvent) => {
        e.stopPropagation();
        try {
            await fetch(`http://127.0.0.1:5002/api/v3/downloadclient/queue/${clientId}/${encodeURIComponent(downloadId)}/resume`, {
                method: 'POST'
            });
            // Optimistic update
            setDownloads(prev => prev.map(d =>
                d.id === downloadId ? { ...d, state: 0 } : d
            ));
        } catch (error) {
            console.error('Error resuming download:', error);
        }
    };

    const handleDelete = async (clientId: number, downloadId: string, e: React.MouseEvent) => {
        e.stopPropagation();
        if (!window.confirm('Are you sure you want to delete this download? This action is aggressive and will try to delete files.')) {
            return;
        }

        try {
            const response = await fetch(`http://127.0.0.1:5002/api/v3/downloadclient/queue/${clientId}/${encodeURIComponent(downloadId)}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                setDownloads(prev => prev.filter(d => d.id !== downloadId));
            } else {
                alert('Failed to delete download');
            }
        } catch (error) {
            console.error('Error deleting download:', error);
            alert('Error deleting download');
        }
    };

    const getStatusLabel = (state: number) => {
        switch (state) {
            case 0: return <span className="status-badge downloading">Downloading</span>;
            case 1: return <span className="status-badge paused">Paused</span>;
            case 2: return <span className="status-badge completed">Completed</span>;
            case 3: return <span className="status-badge error">Error</span>;
            case 4: return <span className="status-badge paused">Queued</span>;
            case 5: return <span className="status-badge downloading">Checking</span>;
            case 7: return <span className="status-badge copying">Copying</span>;
            default: return <span className="status-badge">Unknown</span>;
        }
    };

    const formatSize = (bytes: number) => {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    };

    return (
        <div className="status-page">
            <div className="status-header">
                <h1>Status</h1>
                <p>Active downloads and queue</p>
            </div>

            <div className="downloads-table-container">
                <table className="downloads-table">
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>Size</th>
                            <th>Progress</th>
                            <th>State</th>
                            <th style={{ width: '120px', textAlign: 'right' }}>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
                        {downloads.length === 0 ? (
                            <tr>
                                <td colSpan={5} className="empty-state">
                                    {loading ? 'Loading...' : 'No active downloads'}
                                </td>
                            </tr>
                        ) : (
                            downloads.map((download) => (
                                <tr key={`${download.clientId}-${download.id}`}>
                                    <td style={{ maxWidth: '400px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                        <div style={{ fontWeight: 500 }}>{download.name}</div>
                                        <div className="progress-bar-container">
                                            <div
                                                className="progress-bar-fill"
                                                style={{ width: `${download.progress}%`, backgroundColor: download.state === 3 ? '#ef4444' : download.state === 2 ? '#10b981' : '#3b82f6' }}
                                            />
                                        </div>
                                    </td>
                                    <td>{formatSize(download.size)}</td>
                                    <td>{download.progress.toFixed(1)}%</td>
                                    <td>{getStatusLabel(download.state)}</td>
                                    <td>
                                        <div className="control-actions">
                                            {/* Only show Play/Pause for active states, not completed/error */}
                                            {(download.state === 0 || download.state === 4 || download.state === 5) && (
                                                <button
                                                    className="control-btn"
                                                    onClick={(e) => handlePause(download.clientId, download.id, e)}
                                                    title="Pause"
                                                >
                                                    ⏸
                                                </button>
                                            )}
                                            {download.state === 1 && (
                                                <button
                                                    className="control-btn"
                                                    onClick={(e) => handleResume(download.clientId, download.id, e)}
                                                    title="Resume"
                                                >
                                                    ▶
                                                </button>
                                            )}

                                            <button
                                                className="delete-btn"
                                                onClick={(e) => handleDelete(download.clientId, download.id, e)}
                                                title="Remove download and files"
                                            >
                                                ✕
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
};

export default Status;
