import React, { useState, useEffect } from 'react';
import axios from 'axios';
import { t as translate } from '../i18n/translations';
import './FolderExplorerModal.css';

interface FolderEntry {
    name: string;
    path: string;
    isDirectory: boolean;
}

interface FolderExplorerModalProps {
    initialPath: string;
    onSelect: (path: string) => void;
    onClose: () => void;
    language: any;
}

const FolderExplorerModal: React.FC<FolderExplorerModalProps> = ({ initialPath, onSelect, onClose, language }) => {
    const [currentPath, setCurrentPath] = useState(initialPath || '/');
    const [entries, setEntries] = useState<FolderEntry[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [parentPath, setParentPath] = useState<string | null>(null);

    const t = (key: any) => translate(key, language);

    useEffect(() => {
        loadPath(currentPath);
    }, []);

    const loadPath = async (path: string) => {
        setLoading(true);
        setError(null);
        try {
            const response = await axios.get(`/api/v3/filesystem?path=${encodeURIComponent(path)}`);
            // Map new API data to component state
            // Backend returns: [{ Name, Path, Type }]
            const items = response.data;

            // Derive parent path from backend response check ".."
            const parentItem = items.find((i: any) => i.name === "..");
            setParentPath(parentItem ? parentItem.path : null);

            // Filter out "." and ".." from list for cleaner view
            const contentItems = items.filter((i: any) => i.name !== ".." && i.name !== ".");

            setEntries(contentItems.map((i: any) => ({
                name: i.name,
                path: i.path,
                isDirectory: i.type === 'directory' || i.type === 'drive',
                type: i.type
            })));

            // Update current path to what we requested (or should the backend return resolved path?)
            // The backend doesn't explicitly return "CurrentPath" wrapper object like before. 
            // We assume successful list means path fits.
            setCurrentPath(path);

        } catch (err: any) {
            setError(err.response?.data?.error || err.message);
        } finally {
            setLoading(false);
        }
    };

    const handleBack = () => {
        if (parentPath) {
            loadPath(parentPath);
        } else {
            // Maybe go to root?
            if (currentPath !== "/") loadPath("/");
        }
    };

    const handleEntryClick = (entry: FolderEntry & { type?: string }) => {
        if (entry.isDirectory) {
            loadPath(entry.path);
        } else {
            // If it's a file, maybe double click selects it?
            onSelect(entry.path);
        }
    };

    const handleSelect = () => {
        onSelect(currentPath);
    };

    return (
        <div className="folder-explorer-mask" onClick={onClose}>
            <div className="folder-explorer-modal" onClick={e => e.stopPropagation()}>
                <div className="explorer-header">
                    <h3>{t('selectFolder')}</h3>
                    <button className="close-btn" onClick={onClose}>&times;</button>
                </div>

                <div className="explorer-path-bar">
                    <button className="back-btn" onClick={handleBack} disabled={loading}>
                        ⬅️
                    </button>
                    <input
                        type="text"
                        value={currentPath}
                        readOnly
                        className="path-display"
                    />
                </div>

                <div className="explorer-content">
                    {loading && <div className="explorer-message">Cargando...</div>}
                    {error && <div className="explorer-message error">{error}</div>}
                    {!loading && !error && (
                        <div className="entries-list">
                            {entries.length === 0 && <div className="explorer-message">Carpeta vacía (o acceso denegado).</div>}
                            {entries.map((entry: any) => (
                                <div
                                    key={entry.path}
                                    className={`folder-entry ${entry.type}`}
                                    onDoubleClick={() => handleEntryClick(entry)}
                                    onClick={() => {
                                        // Allow single click selection updates path input? 
                                        // Or just highlight. For now, keep simple.
                                        if (!entry.isDirectory) {
                                            // Optional: Update displayed path if file clicked?
                                            // setCurrentPath(entry.path); // Maybe weird for navigation
                                        }
                                    }}
                                >
                                    <span className="icon">
                                        {entry.type === 'drive' ? '💾' : (entry.isDirectory ? '📁' : '📄')}
                                    </span>
                                    <span className="name">{entry.name}</span>
                                </div>
                            ))}
                        </div>
                    )}
                </div>

                <div className="explorer-footer">
                    <p className="hint">{t('doubleClickToOpen') || 'Doble clic para abrir carpeta'}</p>
                    <div className="actions">
                        <button className="btn-secondary" onClick={onClose}>{t('cancel')}</button>
                        <button className="btn-primary" onClick={handleSelect}>{t('selectCurrent') || 'Seleccionar Actual'}</button>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default FolderExplorerModal;
