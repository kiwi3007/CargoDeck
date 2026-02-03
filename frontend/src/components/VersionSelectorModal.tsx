import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faCodeBranch, faDownload, faHardDrive } from '@fortawesome/free-solid-svg-icons';
import '../pages/Settings.css'; // Reuse modal styles

export interface VersionOption {
    label: string;
    path: string;
    details?: string;
    tag?: string;
}

interface VersionSelectorModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSelect: (path: string) => void;
    options: VersionOption[];
    gameTitle: string;
}

const VersionSelectorModal: React.FC<VersionSelectorModalProps> = ({ isOpen, onClose, onSelect, options, gameTitle }) => {
    if (!isOpen) return null;

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: '500px' }}>
                <div className="modal-header">
                    <h3><FontAwesomeIcon icon={faCodeBranch} /> Select Version</h3>
                    <button className="modal-close" onClick={onClose}>×</button>
                </div>

                <div className="modal-body" style={{ padding: '20px' }}>
                    <p style={{ marginBottom: '15px' }}>
                        Multiple installers found for <strong>{gameTitle}</strong>. Please select which version you want to install:
                    </p>

                    <div className="version-list" style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                        {options.map((opt, idx) => (
                            <button
                                key={idx}
                                className="version-option-btn"
                                onClick={() => onSelect(opt.path)}
                                style={{
                                    background: '#2a2b3d',
                                    border: '1px solid rgba(255,255,255,0.1)',
                                    borderRadius: '8px',
                                    padding: '15px',
                                    textAlign: 'left',
                                    cursor: 'pointer',
                                    display: 'flex',
                                    alignItems: 'center',
                                    transition: 'all 0.2s',
                                    color: '#cdd6f4'
                                }}
                                onMouseOver={e => e.currentTarget.style.background = '#3a3b52'}
                                onMouseOut={e => e.currentTarget.style.background = '#2a2b3d'}
                            >
                                <div style={{ marginRight: '15px', opacity: 0.7 }}>
                                    <FontAwesomeIcon icon={faHardDrive} size="lg" />
                                </div>
                                <div style={{ flex: 1 }}>
                                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                        <span style={{ fontWeight: 'bold', fontSize: '1.1rem' }}>{opt.label}</span>
                                        {opt.tag && (
                                            <span style={{
                                                fontSize: '0.7rem',
                                                padding: '2px 6px',
                                                borderRadius: '4px',
                                                background: opt.tag === 'Installer' ? '#f5c2e7' : '#a6e3a1',
                                                color: '#1e1e2e',
                                                fontWeight: 'bold'
                                            }}>
                                                {opt.tag}
                                            </span>
                                        )}
                                    </div>
                                    {opt.details && (
                                        <div style={{ fontSize: '0.85rem', opacity: 0.6, marginTop: '4px', wordBreak: 'break-all' }}>
                                            {opt.details}
                                        </div>
                                    )}
                                </div>
                                <div style={{ marginLeft: '10px', opacity: 0.5 }}>
                                    <FontAwesomeIcon icon={faDownload} />
                                </div>
                            </button>
                        ))}
                    </div>

                    <div className="modal-actions" style={{ marginTop: '20px', justifyContent: 'center' }}>
                        <button className="btn-secondary" onClick={onClose}>Cancel</button>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default VersionSelectorModal;
