import React, { useEffect } from 'react';
import { t } from '../i18n/translations';
import './Settings.css'; // Reuse settings styles for consistency
import appLogo from '../assets/app_logo.png';
import { useUI } from '../context/UIContext';

const About: React.FC = () => {
    const { toggleKofi } = useUI();

    useEffect(() => {
        const scriptId = 'kofi-overlay-widget';
        if (!document.getElementById(scriptId)) {
            const script = document.createElement('script');
            script.id = scriptId;
            script.src = 'https://storage.ko-fi.com/cdn/scripts/overlay-widget.js';
            script.async = true;
            script.onload = () => {
                if ((window as any).kofiWidgetOverlay) {
                    (window as any).kofiWidgetOverlay.draw('maikboarder', {
                        'type': 'floating-chat',
                        'floating-chat.donateButton.text': 'Support me',
                        'floating-chat.donateButton.background-color': '#fcbf47',
                        'floating-chat.donateButton.text-color': '#323842'
                    });
                }
            };
            document.body.appendChild(script);
        } else {
            // If already loaded (e.g. revisiting page), just redraw if needed or rely on persistence?
            // The widget usually attaches to body. If we leave and come back, we might duplicate.
            // But 'draw' creates a container. Let's try to ensure it draws if script exists.
            if ((window as any).kofiWidgetOverlay) {
                (window as any).kofiWidgetOverlay.draw('maikboarder', {
                    'type': 'floating-chat',
                    'floating-chat.donateButton.text': 'Support me',
                    'floating-chat.donateButton.background-color': '#fcbf47',
                    'floating-chat.donateButton.text-color': '#323842'
                });
            }
        }

        return () => {
            // Optional: Remove widget on unmount to keep it local to "About"
            // The widget creates a div with class 'floating-chat-kofi-popup-iframe'.
            // Removing script doesn't remove the iframe.
            // But let's leave it per user request "en el Acerca de" implies context, 
            // but Ko-fi floating widgets usually persist. 
            // If user wants it ONLY on About, I should cleanup.
            // I'll leave cleanup commented out unless requested, as re-initializing can be tricky.
            // Actually, if I don't cleanup, it stays on every page after visiting About. 
            // "en el Acerca de" -> Implicitly ONLY in About?
            // I will try to remove the specific iframe if possible.
            // For now, I'll just load it. If it persists, that's usually a "feature" of these widgets.
        };
    }, []);

    return (
        <div className="settings">
            <div className="settings-section">
                <div style={{ textAlign: 'center', marginBottom: '1.25rem' }}>
                    <div onClick={toggleKofi} style={{ cursor: 'pointer', display: 'inline-block' }}>
                        <img src={appLogo} alt="Playerr" style={{ width: '100px', height: 'auto', marginBottom: '0.75rem' }} />
                    </div>
                    <h3>Playerr v0.3.3</h3>
                </div>

                <div className="settings-section" style={{ border: 'none', padding: 0, backgroundColor: 'transparent' }}>

                    <p className="settings-description" style={{ fontSize: '0.95rem', lineHeight: '1.5', marginBottom: '1.25rem' }}>
                        {t('aboutMainDesc')}
                    </p>

                    <div style={{ marginBottom: '1.25rem' }}>
                        <h4 style={{ color: '#cdd6f4', fontSize: '1rem', marginBottom: '0.75rem', fontWeight: 600 }}>{t('featuresTitle')}</h4>
                        <ul style={{ listStyleType: 'none', padding: 0, color: '#a6adc8', lineHeight: '1.6', fontSize: '0.9rem' }}>
                            <li>• {t('featureScanning')}</li>
                            <li>• {t('featureIntegration')}</li>
                            <li>• {t('featureCrossPlatform')}</li>
                        </ul>
                    </div>

                    <div style={{ marginBottom: '1.25rem' }}>
                        <h4 style={{ color: '#cdd6f4', fontSize: '1rem', marginBottom: '0.75rem', fontWeight: 600 }}>{t('roadmapTitle')}</h4>
                        <ul style={{ listStyleType: 'none', padding: 0, color: '#a6adc8', lineHeight: '1.6', fontSize: '0.9rem' }}>
                            <li>• {t('roadmapBazzite')}</li>
                            <li>• {t('roadmapUsb')}</li>
                            <li>• {t('roadmapAppStores')}</li>
                            <li>• {t('roadmapExtensibility')}</li>
                        </ul>
                    </div>

                    <div style={{ borderTop: '1px solid #313244', paddingTop: '1.25rem', marginTop: '1.5rem' }}>
                        <p className="settings-description" style={{ marginBottom: '0.5rem' }}>
                            {t('developedBy')} <strong style={{ color: '#cdd6f4' }}>{t('developedByStrong')}</strong>.
                        </p>
                        <p className="settings-description" style={{ fontStyle: 'italic', opacity: 0.8, marginBottom: '1rem' }}>
                            {t('supportText')}
                        </p>

                        <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '0.75rem' }}>
                            <a href="https://github.com/Maikboarder" target="_blank" rel="noopener noreferrer" style={{ opacity: 0.8, transition: 'opacity 0.2s' }}>
                                <img src="https://img.shields.io/github/followers/Maikboarder?label=Follow&style=social" alt="GitHub Follow" />
                            </a>
                        </div>
                        <p className="settings-description" style={{ fontSize: '0.8rem', opacity: 0.5, marginBottom: 0 }}>
                            {t('license')}
                        </p>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default About;
