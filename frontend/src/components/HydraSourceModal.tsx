import React, { useState, useEffect } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSave } from '@fortawesome/free-solid-svg-icons';
import axios from 'axios';
import '../pages/Settings.css';
import { useTranslation } from '../i18n/translations';

interface HydraSourceModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSave: () => void;
    source?: { id?: number; name: string; url: string; enabled: boolean } | null;
}

const HydraSourceModal: React.FC<HydraSourceModalProps> = ({ isOpen, onClose, onSave, source }) => {
    const { t } = useTranslation();
    const [name, setName] = useState('');
    const [url, setUrl] = useState('');
    const [enabled, setEnabled] = useState(true);

    useEffect(() => {
        if (source) {
            setName(source.name);
            setUrl(source.url);
            setEnabled(source.enabled);
        } else {
            setName('');
            setUrl('');
            setEnabled(true);
        }
    }, [source, isOpen]);

    const handleSave = async (e: React.FormEvent) => {
        e.preventDefault();
        try {
            const payload = { name, url, enabled };

            if (source && source.id) {
                await axios.put(`/api/v3/hydra/${source.id}`, payload);
            } else {
                await axios.post('/api/v3/hydra', payload);
            }
            onSave();
            onClose();
        } catch (error: any) {
            alert(`Error saving source: ${error.message}`);
        }
    };

    if (!isOpen) return null;

    return (
        <div className="modal-overlay" onClick={onClose}>
            <div className="modal" onClick={(e) => e.stopPropagation()}>
                <div className="modal-header">
                    <h3>{source ? t('editHydraSource') : t('addHydraSource')}</h3>
                    <button className="modal-close" onClick={onClose}>×</button>
                </div>
                <form onSubmit={handleSave}>
                    <div className="form-group">
                        <label>{t('name')}</label>
                        <input
                            type="text"
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            placeholder={t('clientNamePlaceholder')}
                            className="form-control"
                            required
                        />
                    </div>

                    <div className="form-group">
                        <label>{t('sourceUrl')}</label>
                        <input
                            type="url"
                            value={url}
                            onChange={(e) => setUrl(e.target.value)}
                            placeholder={t('sourceUrlPlaceholder')}
                            className="form-control"
                            required
                        />
                        <small>{t('hydraJsonHelp')}</small>
                    </div>

                    <div className="modal-actions">
                        <button type="button" className="btn-secondary" onClick={onClose}>{t('cancel')}</button>
                        <button type="submit" className="btn-primary">
                            {t('save')}
                        </button>
                    </div>
                </form>
            </div>
        </div>
    );
};

export default HydraSourceModal;
