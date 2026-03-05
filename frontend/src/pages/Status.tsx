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
    clientName: string;
}

const Status: React.FC = () => {
    const { t } = useTranslation();
    const [downloads, setDownloads] = useState<DownloadStatus[]>([]);
    const [loading, setLoading] = useState(true);
    const [deleteCandidate, setDeleteCandidate] = useState<{ clientId: number; id: string; name: string } | null>(null);

    const fetchQueue = async () => {
        try {
            const response = await fetch('/api/v3/downloadclient/queue');
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
        fetchQueue(); // initial load before first SSE push
        const handler = (e: Event) => {
            try {
                const data = JSON.parse((e as CustomEvent).detail);
                setDownloads(data || []);
                setLoading(false);
            } catch { /* ignore */ }
        };
        window.addEventListener('DOWNLOAD_QUEUE_UPDATED_EVENT', handler);
        return () => window.removeEventListener('DOWNLOAD_QUEUE_UPDATED_EVENT', handler);
    }, []);

    const handlePause = async (clientId: number, downloadId: string, e: React.MouseEvent) => {
        e.stopPropagation();
        try {
            await fetch(`/api/v3/downloadclient/queue/${clientId}/${encodeURIComponent(downloadId)}/pause`, {
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
            await fetch(`/api/v3/downloadclient/queue/${clientId}/${encodeURIComponent(downloadId)}/resume`, {
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

    const handleDeleteClick = (clientId: number, downloadId: string, name: string, e: React.MouseEvent) => {
        e.stopPropagation();
        setDeleteCandidate({ clientId, id: downloadId, name });
    };

    const confirmDelete = async () => {
        if (!deleteCandidate) return;

        try {
            const response = await fetch(`/api/v3/downloadclient/queue/${deleteCandidate.clientId}/${encodeURIComponent(deleteCandidate.id)}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                setDownloads(prev => prev.filter(d => d.id !== deleteCandidate.id));
                setDeleteCandidate(null);
            } else {
                alert(t('failedToDeleteDownload'));
            }
        } catch (error) {
            console.error('Error deleting download:', error);
            alert(t('failedToDeleteDownload'));
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
                <h1>{t('statusPageTitle')}</h1>
                <p>{t('statusPageDesc')}</p>
            </div>

            <div className="downloads-table-container">
                <table className="downloads-table">
                    <thead>
                        <tr>
                            <th>{t('client')}</th>
                            <th>{t('name')}</th>
                            <th>{t('size')}</th>
                            <th>{t('progress')}</th>
                            <th>{t('state')}</th>
                            <th style={{ width: '120px', textAlign: 'right' }}>{t('actions')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {downloads.length === 0 ? (
                            <tr>
                                <td colSpan={6} className="empty-state">
                                    {loading ? t('loading') : t('noActiveDownloads')}
                                </td>
                            </tr>
                        ) : (
                            downloads.map((download) => (
                                <tr key={`${download.clientId}-${download.id}`}>
                                    <td>
                                        <span className="client-badge">{download.clientName}</span>
                                    </td>
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
                                                onClick={(e) => handleDeleteClick(download.clientId, download.id, download.name, e)}
                                                title={t('remove')}
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


            {
                deleteCandidate && (
                    <div className="modal-overlay" onClick={() => setDeleteCandidate(null)}>
                        <div className="modal" onClick={(e) => e.stopPropagation()}>
                            <div className="modal-header">
                                <h3>{t('deleteDownload') || 'Delete Download'}</h3>
                                <button className="modal-close" onClick={() => setDeleteCandidate(null)}>×</button>
                            </div>
                            <div className="modal-content">
                                <p>
                                    {t('confirmDeleteDownload') || 'Are you sure you want to delete this download?'}: <br />
                                    <br />
                                    <strong>{deleteCandidate.name}</strong>
                                </p>
                            </div>
                            <div className="modal-actions">
                                <button className="btn-secondary" onClick={() => setDeleteCandidate(null)}>{t('cancel')}</button>
                                <button className="btn-danger-modal" onClick={confirmDelete}>{t('delete')}</button>
                            </div>
                        </div>
                    </div>
                )
            }
        </div >
    );
};

export default Status;
