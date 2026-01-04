import React from 'react';
import { t } from '../i18n/translations';
import './Settings.css'; // Reuse settings styles for consistency
import appLogo from '../assets/app_logo.png';

const About: React.FC = () => {
    React.useEffect(() => {
        const script = document.createElement('script');
        script.src = 'https://storage.ko-fi.com/cdn/scripts/overlay-widget.js';
        script.async = true;
        script.onload = () => {
            // @ts-ignore
            if (window.kofiWidgetOverlay) {
                // @ts-ignore
                window.kofiWidgetOverlay.draw('maikboarder', {
                    'type': 'floating-chat',
                    'floating-chat.donateButton.text': 'Support me',
                    'floating-chat.donateButton.background-color': '#fcbf47',
                    'floating-chat.donateButton.text-color': '#323842'
                });
            }
        };
        document.body.appendChild(script);

        return () => {
            // Cleanup script on unmount to prevent duplicates if user navigates away and back
            if (document.body.contains(script)) {
                try {
                    document.body.removeChild(script);
                } catch (e) {
                    // Ignore removal errors
                }
            }
            // Ideally we should also remove the widget DOM element created by the script, 
            // but the ko-fi script doesn't expose a clean destroy method easily.
            // For now, removing the script prevents re-execution.
            // To be safe, we can try to remove the iframe/div it creates if we knew the ID.
            // Ko-fi creates a div with id 'kofi-widget-overlay-container' usually.
            const widget = document.getElementById('kofi-widget-overlay-container');
            if (widget) {
                widget.remove();
            }
        };
    }, []);

    return (
        <div className="settings">
            <div className="settings-section">
                <div style={{ textAlign: 'center', marginBottom: '1.25rem' }}>
                    <img src={appLogo} alt="Playerr" style={{ width: '100px', height: 'auto', marginBottom: '0.75rem' }} />
                    <h3>Playerr v0.1.1 (Beta)</h3>
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
