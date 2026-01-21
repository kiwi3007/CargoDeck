import React, { useState, useEffect } from 'react';
import axios from 'axios';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faMicrochip, faDownload, faCheckCircle, faSpinner, faExclamationTriangle, faInfoCircle } from '@fortawesome/free-solid-svg-icons';
import '../pages/Settings.css';

interface SwitchInstallerModalProps {
    isOpen: boolean;
    onClose: () => void;
    filePath: string;
    fileName: string;
}

const SwitchInstallerModal: React.FC<SwitchInstallerModalProps> = ({ isOpen, onClose, filePath, fileName }) => {
    const [step, setStep] = useState<'scan' | 'confirm' | 'installing' | 'finished' | 'aborted' | 'error'>('scan');
    const [isInstalling, setIsInstalling] = useState(false);
    const [devices, setDevices] = useState<string[]>([]);
    const [selectedDevice, setSelectedDevice] = useState<string>('');
    const [progress, setProgress] = useState(0);
    const [log, setLog] = useState<string>('');

    useEffect(() => {
        if (isOpen) {
            setStep('scan');
            setDevices([]);
            setLog('');
            setIsInstalling(false);
            scanDevices();
        }
    }, [isOpen]);

    const handleClose = async () => {
        if (isInstalling) {
            try {
                await axios.post('/api/v3/nsw/cancel');
            } catch (e) {
                console.error('Cancel error:', e);
            }
        }
        onClose();
    };

    const scanDevices = async () => {
        try {
            setLog('Scanning for USB devices...');
            const res = await axios.get('/api/v3/nsw/devices');
            const foundDevices = res.data;
            setDevices(foundDevices);
            if (foundDevices.length > 0) {
                setLog(`Found ${foundDevices.length} device(s).`);
                setSelectedDevice(foundDevices[0]);
                setStep('confirm');
            } else {
                setLog('No Nintendo Switch detected defined by VID 057E.');
            }
        } catch (err: any) {
            setLog(`Error scanning: ${err.message}`);
        }
    };

    const startInstall = async () => {
        if (!selectedDevice) return;
        setStep('installing');
        setIsInstalling(true);
        setProgress(0);
        setLog('Starting handshake...');

        try {
            // Trigger install
            await axios.post('/api/v3/nsw/install', {
                filePath,
                deviceId: selectedDevice
            });

            // Poll real progress from backend
            const interval = setInterval(async () => {
                try {
                    const statusRes = await axios.get('/api/v3/nsw/progress');
                    const { progress: currentP, status: currentS } = statusRes.data;

                    setProgress(Math.round(currentP));
                    setLog(currentS);

                    if (currentS === 'Installation Complete' || currentP >= 100) {
                        clearInterval(interval);
                        setStep('finished');
                        setIsInstalling(false);
                    } else if (currentS === 'Installation Aborted by Console') {
                        clearInterval(interval);
                        setStep('aborted');
                        setLog('Installation was cancelled from the Switch.');
                        setIsInstalling(false);
                        // Auto close after 2 seconds
                        setTimeout(() => {
                            onClose();
                        }, 2000);
                    } else if (currentS.startsWith('Error')) {
                        clearInterval(interval);
                        setStep('error');
                        setLog(currentS);
                        setIsInstalling(false);
                    }
                } catch (pollErr: any) {
                    console.error('Progress polling error:', pollErr);
                }
            }, 1000);

        } catch (err: any) {
            setStep('error');
            setLog('Installation failed: ' + err.message);
            setIsInstalling(false);
        }
    };

    if (!isOpen) return null;

    return (
        <div className="modal-overlay" onClick={handleClose}>
            <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: '500px' }}>
                <div className="modal-header">
                    <h3><FontAwesomeIcon icon={faMicrochip} /> Install to Switch</h3>
                    <button className="modal-close" onClick={handleClose}>×</button>
                </div>

                <div className="modal-body" style={{ padding: '20px' }}>

                    {step === 'scan' && (
                        <div style={{ textAlign: 'center', padding: '20px' }}>
                            <FontAwesomeIcon icon={faSpinner} spin size="3x" />
                            <p style={{ marginTop: '15px' }}>Scanning for Nintendo Switch via USB...</p>
                            <small>{log}</small>
                            <button className="btn-secondary" onClick={scanDevices} style={{ marginTop: '10px' }}>Retry</button>
                        </div>
                    )}

                    {step === 'confirm' && (
                        <div>
                            <p><strong>File:</strong> {fileName}</p>
                            <div className="form-group">
                                <label>Target Device:</label>
                                <select
                                    className="form-control"
                                    value={selectedDevice}
                                    onChange={e => setSelectedDevice(e.target.value)}
                                >
                                    {devices.map((d, i) => <option key={i} value={d}>{d}</option>)}
                                </select>
                            </div>
                            <p className="info-text">
                                Ensure Tinfoil or DBI is running on the Switch and connected via USB.
                            </p>
                            <div className="modal-actions">
                                <button className="btn-primary" onClick={startInstall}>
                                    <FontAwesomeIcon icon={faDownload} /> Install Now
                                </button>
                            </div>
                        </div>
                    )}

                    {step === 'installing' && (
                        <div style={{ textAlign: 'center' }}>
                            <h4>Installing...</h4>
                            <div className="progress-bar-container" style={{ background: '#333', height: '20px', borderRadius: '10px', margin: '20px 0', overflow: 'hidden' }}>
                                <div style={{ width: `${progress}%`, background: '#4CAF50', height: '100%', transition: 'width 0.3s' }}></div>
                            </div>
                            <small>{log}</small>
                        </div>
                    )}

                    {step === 'finished' && (
                        <div style={{ textAlign: 'center', color: '#4CAF50' }}>
                            <FontAwesomeIcon icon={faCheckCircle} size="4x" />
                            <h3>Success!</h3>
                            <p>Game installed successfully.</p>
                            <button className="btn-secondary" onClick={onClose}>Close</button>
                        </div>
                    )}

                    {step === 'aborted' && (
                        <div style={{ textAlign: 'center', color: '#aaa' }}>
                            <FontAwesomeIcon icon={faInfoCircle} size="3x" />
                            <h3>Cancelled</h3>
                            <p>{log}</p>
                        </div>
                    )}

                    {step === 'error' && (
                        <div style={{ textAlign: 'center', color: '#ff5555' }}>
                            <FontAwesomeIcon icon={faExclamationTriangle} size="3x" />
                            <h3>Error</h3>
                            <p>{log}</p>
                            <button className="btn-secondary" onClick={onClose}>Close</button>
                        </div>
                    )}

                </div>
            </div>
        </div>
    );
};

export default SwitchInstallerModal;
