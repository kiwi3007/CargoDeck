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
            const response = await axios.get(`/api/v3/explore?path=${encodeURIComponent(path)}`);
            setEntries(response.data.entries);
            setCurrentPath(response.data.currentPath);
            setParentPath(response.data.parentPath);
        } catch (err: any) {
            setError(err.response?.data?.error || err.message);
        } finally {
            setLoading(false);
        }
    };

    const handleBack = () => {
        if (parentPath) {
            loadPath(parentPath);
        }
    };

    const handleEntryClick = (entry: FolderEntry) => {
        if (entry.isDirectory) {
            loadPath(entry.path);
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
                    <button className="back-btn" onClick={handleBack} disabled={!parentPath || loading}>
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
                            {entries.length === 0 && <div className="explorer-message">No hay carpetas.</div>}
                            {entries.map(entry => (
                                <div
                                    key={entry.path}
                                    className="folder-entry"
                                    onDoubleClick={() => handleEntryClick(entry)}
                                    onClick={(e) => {
                                        // Mobile friendly: single click to select/open?
                                        // For now, let's keep it simple.
                                    }}
                                >
                                    <span className="icon">📁</span>
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
