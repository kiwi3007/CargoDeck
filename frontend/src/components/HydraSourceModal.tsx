import React, { useState, useEffect } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSave } from '@fortawesome/free-solid-svg-icons';
import axios from 'axios';
import '../pages/Settings.css';

interface HydraSourceModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSave: () => void;
    source?: { id?: number; name: string; url: string; enabled: boolean } | null;
}

const HydraSourceModal: React.FC<HydraSourceModalProps> = ({ isOpen, onClose, onSave, source }) => {
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
                    <h3>{source ? 'Edit Hydra Source' : 'Add Hydra Source'}</h3>
                    <button className="modal-close" onClick={onClose}>×</button>
                </div>
                <form onSubmit={handleSave}>
                    <div className="form-group">
                        <label>Name</label>
                        <input
                            type="text"
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            placeholder="e.g. FitGirl Repacks"
                            className="form-control"
                            required
                        />
                    </div>

                    <div className="form-group">
                        <label>Source URL (JSON)</label>
                        <input
                            type="url"
                            value={url}
                            onChange={(e) => setUrl(e.target.value)}
                            placeholder="https://example.com/sources.json"
                            className="form-control"
                            required
                        />
                        <small>Must be a valid Hydra-compatible JSON URL.</small>
                    </div>



                    <div className="modal-actions">
                        <button type="button" className="btn-secondary" onClick={onClose}>Cancel</button>
                        <button type="submit" className="btn-primary">
                            Save
                        </button>
                    </div>
                </form>
            </div>
        </div>
    );
};

export default HydraSourceModal;
