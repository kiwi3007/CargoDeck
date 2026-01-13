import React, { useState } from 'react';
import axios from 'axios';
import { t as translate } from '../i18n/translations';
import FolderExplorerModal from './FolderExplorerModal';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch } from '@fortawesome/free-solid-svg-icons';
import './GameCorrectionModal.css';

interface GameCorrectionModalProps {
    game: any;
    onClose: () => void;
    onSave: (updates: any) => void;
    language?: string;
}

const GameCorrectionModal: React.FC<GameCorrectionModalProps> = ({ game, onClose, onSave, language = 'es' }) => {
    const [activeTab, setActiveTab] = useState<'metadata' | 'path' | 'playPath'>('metadata');

    // Metadata State
    const [searchTerm, setSearchTerm] = useState(game.title);
    const [results, setResults] = useState<any[]>([]);
    const [searching, setSearching] = useState(false);
    const [selectedMetadata, setSelectedMetadata] = useState<any | null>(null);

    // Path State
    const [installPath, setInstallPath] = useState(game.installPath || game.path || '');
    const [executablePath, setExecutablePath] = useState(game.executablePath || '');
    const [showFileExplorer, setShowFileExplorer] = useState(false);
    const [explorerMode, setExplorerMode] = useState<'install' | 'executable'>('install');

    const t = (key: string) => translate(key as any, language as any);

    const handleSearch = async () => {
        if (!searchTerm) return;
        setSearching(true);
        try {
            const response = await axios.get('/api/v3/game/lookup', {
                params: { term: searchTerm, lang: language }
            });
            setResults(response.data);
        } catch (error) {
            console.error(error);
        } finally {
            setSearching(false);
        }
    };

    const handleSave = () => {
        const updates: any = {};
        if (selectedMetadata) {
            updates.igdbId = selectedMetadata.igdbId;
            updates.title = selectedMetadata.title; // Optional: Update title immediately
        }
        if (installPath !== game.installPath) {
            updates.installPath = installPath;
        }
        if (executablePath !== game.executablePath) {
            updates.executablePath = executablePath;
        }

        onSave(updates);
    };

    const openExplorer = (mode: 'install' | 'executable') => {
        setExplorerMode(mode);
        setShowFileExplorer(true);
    };

    return (
        <div className="correction-modal-mask">
            <div className="correction-modal">
                <div className="modal-header">
                    <h3>{t('correctGame') || 'Corregir Juego'}</h3>
                    <button className="close-btn" onClick={onClose}>&times;</button>
                </div>

                <div className="modal-tabs">
                    <button
                        className={`tab-btn ${activeTab === 'metadata' ? 'active' : ''}`}
                        onClick={() => setActiveTab('metadata')}
                    >
                        {t('metadata') || 'Metadatos'}
                    </button>
                    <button
                        className={`tab-btn ${activeTab === 'path' ? 'active' : ''}`}
                        onClick={() => setActiveTab('path')}
                    >
                        {t('installPath') || 'Ruta Instalación'}
                    </button>
                    <button
                        className={`tab-btn ${activeTab === 'playPath' ? 'active' : ''}`}
                        onClick={() => setActiveTab('playPath')}
                    >
                        {t('playPath') || 'Play Path'}
                    </button>
                </div>

                <div className="modal-content">
                    {activeTab === 'metadata' && (
                        <div className="metadata-correction">
                            <div className="search-bar">
                                <input
                                    type="text"
                                    value={searchTerm}
                                    onChange={(e) => setSearchTerm(e.target.value)}
                                    placeholder={t('searchGame') || 'Buscar juego...'}
                                    onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                                />
                                <button onClick={handleSearch} disabled={searching} className="search-btn">
                                    {searching ? '...' : <FontAwesomeIcon icon={faSearch} />}
                                </button>
                            </div>

                            <div className="search-results">
                                {results.map((res: any) => (
                                    <div
                                        key={res.igdbId}
                                        className={`search-result-item ${selectedMetadata?.igdbId === res.igdbId ? 'selected' : ''}`}
                                        onClick={() => setSelectedMetadata(res)}
                                    >
                                        <div className="poster">
                                            {res.images?.coverUrl ? <img src={res.images.coverUrl} alt="" /> : '📷'}
                                        </div>
                                        <div className="info">
                                            <div className="title">{res.title}</div>
                                            <div className="year">{res.year}</div>
                                            <div className="id">IGDB: {res.igdbId}</div>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}

                    {activeTab === 'path' && (
                        <div className="path-correction">
                            <label>{t('currentPath') || 'Ruta Actual'}:</label>
                            <div className="path-input-group">
                                <input
                                    type="text"
                                    value={installPath}
                                    onChange={(e) => setInstallPath(e.target.value)}
                                />
                                <button onClick={() => openExplorer('install')}>📁</button>
                            </div>
                            <p className="hint">
                                {t('pathHint') || 'Selecciona la carpeta donde está instalado el juego.'}
                            </p>
                        </div>
                    )}

                    {activeTab === 'playPath' && (
                        <div className="path-correction">
                            <label>{t('executablePath') || 'Ejecutable'}:</label>
                            <div className="path-input-group">
                                <input
                                    type="text"
                                    value={executablePath}
                                    onChange={(e) => setExecutablePath(e.target.value)}
                                    placeholder="/path/to/game.exe"
                                />
                                <button onClick={() => openExplorer('executable')}>📁</button>
                            </div>
                            <p className="hint">
                                {t('playPathHint') || 'Selecciona el archivo ejecutable del juego.'}
                            </p>
                        </div>
                    )}
                </div>

                <div className="modal-footer">
                    <button className="btn-secondary" onClick={onClose}>{t('cancel')}</button>
                    <button className="btn-primary" onClick={handleSave}>{t('save')}</button>
                </div>
            </div>

            {showFileExplorer && (
                <FolderExplorerModal
                    initialPath={explorerMode === 'executable' ? (executablePath || installPath || '/') : (installPath || '/')}
                    language={language}
                    onClose={() => setShowFileExplorer(false)}
                    onSelect={(path) => {
                        if (explorerMode === 'install') {
                            setInstallPath(path);
                        } else {
                            setExecutablePath(path);
                        }
                        setShowFileExplorer(false);
                    }}
                />
            )}
        </div>
    );
};

export default GameCorrectionModal;
