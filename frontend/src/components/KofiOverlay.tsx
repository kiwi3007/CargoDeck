import React from 'react';
import { useUI } from '../context/UIContext';
import './KofiOverlay.css';

const KofiOverlay: React.FC = () => {
    const { isKofiOpen, closeKofi } = useUI();

    if (!isKofiOpen) return null;

    return (
        <div className="kofi-overlay-backdrop" onClick={closeKofi}>
            <div className="kofi-overlay-content" onClick={(e) => e.stopPropagation()}>
                <button className="kofi-close-btn" onClick={closeKofi}>&times;</button>
                <iframe
                    id='kofiframe'
                    src='https://ko-fi.com/maikboarder/?hidefeed=true&widget=true&embed=true&preview=true'
                    style={{ border: 'none', width: '100%', padding: '4px', background: '#f9f9f9', height: '100%', borderRadius: '8px' }}
                    title='maikboarder'
                ></iframe>
            </div>
        </div>
    );
};

export default KofiOverlay;
