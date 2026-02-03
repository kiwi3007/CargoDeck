import React, { useEffect } from 'react';
import { t } from '../i18n/translations';
import './Settings.css'; // Reuse settings styles for consistency
import appLogo from '../assets/app_logo.png';
import kofiLogo from '../assets/kofi_symbol.png';
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
                    <h3>Playerr v0.4.8</h3>
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
                                <img
                                    src={kofiLogo}
                                    alt="Ko-fi"
                                    style={{ width: '22px', height: 'auto', marginRight: '8px' }}
                                />
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
