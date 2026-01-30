import React, { useEffect } from 'react';
import { t } from '../i18n/translations';
import './Settings.css'; // Reuse settings styles for consistency
import appLogo from '../assets/app_logo.png';
import { useUI } from '../context/UIContext';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faGithub } from '@fortawesome/free-brands-svg-icons';
import { faHeart } from '@fortawesome/free-solid-svg-icons';

const About: React.FC = () => {
    const { toggleKofi } = useUI();

    // Removed floating widget script to avoid rendering issues on Windows

    return (
        <div className="settings">
            <div className="settings-section">
                <div style={{ textAlign: 'center', marginBottom: '1.25rem' }}>
                    <div onClick={toggleKofi} style={{ cursor: 'pointer', display: 'inline-block' }}>
                        <img src={appLogo} alt="Playerr" style={{ width: '100px', height: 'auto', marginBottom: '0.75rem' }} />
                    </div>
                    <h3>Playerr v0.4.7</h3>
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
                            <li>• {t('featureUsb')}</li>
                            <li>• {t('featureCrossPlatform')}</li>
                        </ul>
                    </div>

                    <div style={{ marginBottom: '1.25rem' }}>
                        <h4 style={{ color: '#cdd6f4', fontSize: '1rem', marginBottom: '0.75rem', fontWeight: 600 }}>{t('roadmapTitle')}</h4>
                        <ul style={{ listStyleType: 'none', padding: 0, color: '#a6adc8', lineHeight: '1.6', fontSize: '0.9rem' }}>
                            <li>• {t('roadmapBazzite')}</li>
                            {/* <li>• {t('roadmapUsb')} (Completed v0.4.5)</li> */}
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
                            <a
                                href="https://github.com/Maikboarder"
                                target="_blank"
                                rel="noopener noreferrer"
                                className="social-btn github"
                            >
                                <FontAwesomeIcon icon={faGithub} />
                                GitHub
                            </a>
                            <a
                                href="https://ko-fi.com/maikboarder"
                                target="_blank"
                                rel="noopener noreferrer"
                                className="social-btn kofi"
                            >
                                <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18" style={{ marginRight: '6px' }}>
                                    <path d="M23.881 8.948c-.773-4.085-4.859-5.005-4.859-5.005s-.561-1.184-1.253-2.486c-.543-1.023-1.058-1.511-1.637-1.457-.579.054-1.132.55-1.132 1.517 0 .967-.008 5.731-.008 5.731v1.6l-2.008-1.442h-3.955c-1.312 0-2.373 1.061-2.373 2.373v8.527c0 1.312 1.061 2.373 2.373 2.373h10.437c1.312 0 2.373-1.061 2.373-2.373v-1.132c1.391-.129 3.033-1.121 3.518-3.327.484-2.206-.476-4.904-1.476-6.904zm-3.52 6.848c-.223 1.012-1 1.637-2.153 1.637h-1.34v-7.39h1.34c1.153 0 1.93.625 2.153 1.637.223 1.012.223 3.104 0 4.116z" />
                                </svg>
                                Ko-fi
                            </a>
                            <a
                                href="https://github.com/sponsors/Maikboarder"
                                target="_blank"
                                rel="noopener noreferrer"
                                className="social-btn sponsor"
                            >
                                <FontAwesomeIcon icon={faHeart} style={{ color: '#ffffff' }} />
                                Sponsor
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
