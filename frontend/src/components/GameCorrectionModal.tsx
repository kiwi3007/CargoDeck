import React, { useState, useEffect } from 'react';
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
}

const GameCorrectionModal: React.FC<GameCorrectionModalProps> = ({ game, onClose, onSave }) => {
    const [activeTab, setActiveTab] = useState<'metadata' | 'path'>('metadata');

    // Metadata State
    const [searchTerm, setSearchTerm] = useState(game.title);
    const [results, setResults] = useState<any[]>([]);
    const [searching, setSearching] = useState(false);
    const [selectedMetadata, setSelectedMetadata] = useState<any | null>(null);
    const [steamId, setSteamId] = useState<string>(game.steamId?.toString() || '');

    // Path State
    const [gamePath, setGamePath] = useState(game.path || '');
    const [showFileExplorer, setShowFileExplorer] = useState(false);

    // Per-device run settings: agentId → { launchArgs, envVars, protonPath }
    const [agents, setAgents] = useState<any[]>([]);
    const [agentLaunchArgs, setAgentLaunchArgs] = useState<Record<string, string>>({});
    const [agentEnvVars, setAgentEnvVars] = useState<Record<string, string>>({});
    const [agentProtonPath, setAgentProtonPath] = useState<Record<string, string>>({});
    const [savedAgentSettings, setSavedAgentSettings] = useState<Record<string, { launchArgs: string; envVars: string; protonPath: string }>>({});

    useEffect(() => {
        if (!game.id) return;
        Promise.all([
            axios.get('/api/v3/agent'),
            axios.get(`/api/v3/game/${game.id}/agent-launch-args`),
        ]).then(([agentsRes, argsRes]) => {
            setAgents(agentsRes.data || []);
            const settings: Record<string, { launchArgs: string; envVars: string; protonPath: string }> = argsRes.data || {};
            const launchArgs: Record<string, string> = {};
            const envVars: Record<string, string> = {};
            const protonPaths: Record<string, string> = {};
            Object.entries(settings).forEach(([id, s]) => {
                launchArgs[id] = s.launchArgs || '';
                envVars[id] = s.envVars || '';
                protonPaths[id] = s.protonPath || '';
            });
            setAgentLaunchArgs(launchArgs);
            setAgentEnvVars(envVars);
            setAgentProtonPath(protonPaths);
            setSavedAgentSettings(settings);
        }).catch(() => {});
    }, [game.id]);

const t = (key: string) => translate(key as any);

    const handleSearch = async () => {
        if (!searchTerm) return;
        setSearching(true);
        try {
            const response = await axios.get('/api/v3/game/lookup', {
                params: { term: searchTerm, lang: 'en' }
            });
            setResults(response.data);
        } catch (error) {
            console.error(error);
        } finally {
            setSearching(false);
        }
    };

    const handleSave = async () => {
        const updates: any = {};
        if (selectedMetadata) {
            updates.igdbId = selectedMetadata.igdbId;
            updates.title = selectedMetadata.title;
        }
        if (gamePath !== (game.path || '')) {
            updates.path = gamePath;
        }
        const parsedSteamId = parseInt(steamId, 10);
        if (!isNaN(parsedSteamId) && parsedSteamId !== (game.steamId || 0)) {
            updates.steamId = parsedSteamId;
        }

        // Save changed per-device run settings (launch args + env vars + proton path)
        const allAgentIds = new Set([
            ...Object.keys(agentLaunchArgs),
            ...Object.keys(agentEnvVars),
            ...Object.keys(agentProtonPath),
            ...Object.keys(savedAgentSettings),
        ]);
        const patches = Array.from(allAgentIds).filter(agentId => {
            const saved = savedAgentSettings[agentId];
            const newArgs = agentLaunchArgs[agentId] ?? '';
            const newEnv = agentEnvVars[agentId] ?? '';
            const newProton = agentProtonPath[agentId] ?? '';
            return newArgs !== (saved?.launchArgs ?? '') || newEnv !== (saved?.envVars ?? '') || newProton !== (saved?.protonPath ?? '');
        });
        await Promise.all(
            patches.map(agentId =>
                axios.patch(`/api/v3/game/${game.id}/agent-launch-args`, {
                    agentId,
                    launchArgs: agentLaunchArgs[agentId] ?? '',
                    envVars: agentEnvVars[agentId] ?? '',
                    protonPath: agentProtonPath[agentId] ?? '',
                })
            )
        );

        onSave(updates);
    };

    const openExplorer = () => {
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
                        Path
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

                            <div className="steam-id-row">
                                <label>Steam App ID</label>
                                <input
                                    type="number"
                                    value={steamId}
                                    onChange={(e) => setSteamId(e.target.value)}
                                    placeholder="e.g. 646570"
                                    className="steam-id-input"
                                />
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
                            <label>Game folder:</label>
                            <div className="path-input-group">
                                <input
                                    type="text"
                                    value={gamePath}
                                    onChange={(e) => setGamePath(e.target.value)}
                                    placeholder="/path/to/game/folder"
                                />
                                <button onClick={() => openExplorer()}>📁</button>
                            </div>
                            <p className="hint">
                                The folder where game files are stored on this server. Used for the file browser and agent downloads.
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
                    initialPath={gamePath || '/'}
                    onClose={() => setShowFileExplorer(false)}
                    onSelect={(path) => {
                        setGamePath(path);
                        setShowFileExplorer(false);
                    }}
                />
            )}
        </div>
    );
};

export default GameCorrectionModal;
