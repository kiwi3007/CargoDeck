import React from 'react';
import { useUI } from '../context/UIContext';
import './KofiOverlay.css';

const KofiOverlay: React.FC = () => {
    const { isKofiOpen, closeKofi } = useUI();
    const [view, setView] = React.useState<'menu' | 'kofi'>('menu');

    // Reset view when opening
    React.useEffect(() => {
        if (isKofiOpen) setView('menu');
    }, [isKofiOpen]);

    if (!isKofiOpen) return null;

    return (
        <div className="kofi-overlay-backdrop" onClick={closeKofi}>
            <div className="kofi-overlay-content" onClick={(e) => e.stopPropagation()} style={{ height: view === 'menu' ? 'auto' : '720px', padding: view === 'menu' ? '2rem' : '0' }}>
                <button className="kofi-close-btn" onClick={closeKofi}>&times;</button>

                {view === 'menu' ? (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem', alignItems: 'center', color: '#cdd6f4' }}>
                        <h3 style={{ margin: 0, fontSize: '1.5rem' }}>Choose Support Method</h3>
                        <p style={{ textAlign: 'center', opacity: 0.8, marginTop: '-0.5rem' }}>
                            Thank you for supporting Playerr! ❤️
                        </p>

                        <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: '1rem', width: '100%' }}>
                            <a
                                href="https://github.com/sponsors/Maikboarder"
                                target="_blank"
                                rel="noopener noreferrer"
                                className="support-btn github"
                                style={{
                                    display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '10px',
                                    padding: '1rem', background: '#24292e', color: 'white',
                                    textDecoration: 'none', borderRadius: '8px', fontWeight: 'bold',
                                    border: '1px solid rgba(255,255,255,0.1)', transition: 'transform 0.2s'
                                }}
                            >
                                <svg height="24" viewBox="0 0 16 16" version="1.1" width="24" aria-hidden="true" fill="currentColor"><path fillRule="evenodd" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"></path></svg>
                                GitHub Sponsors
                            </a>

                            <button
                                onClick={() => setView('kofi')}
                                className="support-btn kofi"
                                style={{
                                    display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '10px',
                                    padding: '1rem', background: '#fcbf47', color: '#323842',
                                    border: 'none', borderRadius: '8px', fontWeight: 'bold', cursor: 'pointer',
                                    fontSize: '1rem'
                                }}
                            >
                                <span style={{ fontSize: '1.2rem' }}>☕</span>
                                Support on Ko-fi
                            </button>
                        </div>
                    </div>
                ) : (
                    <>
                        <button
                            onClick={() => setView('menu')}
                            style={{
                                position: 'absolute', top: '10px', left: '10px', zIndex: 10,
                                background: 'rgba(0,0,0,0.5)', border: 'none', color: 'white',
                                padding: '5px 10px', borderRadius: '4px', cursor: 'pointer'
                            }}
                        >
                            &larr; Back
                        </button>
                        <iframe
                            id='kofiframe'
                            src='https://ko-fi.com/maikboarder/?hidefeed=true&widget=true&embed=true&preview=true'
                            style={{ border: 'none', width: '100%', padding: '4px', background: '#f9f9f9', height: '100%', borderRadius: '8px', paddingTop: '40px' }}
                            title='maikboarder'
                        ></iframe>
                    </>
                )}
            </div>
        </div>
    );
};

export default KofiOverlay;
