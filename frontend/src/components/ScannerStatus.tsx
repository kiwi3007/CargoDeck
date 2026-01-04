import React, { useState, useEffect, useRef } from 'react';
import axios from 'axios';
import './ScannerStatus.css';

const ScannerStatus: React.FC = () => {
    const [status, setStatus] = useState<{
        isScanning: boolean;
        lastGameFound: string | null;
        gamesAddedCount: number;
    }>({
        isScanning: false,
        lastGameFound: null,
        gamesAddedCount: 0
    });
    const [showFinished, setShowFinished] = useState(false);
    const prevIsScanning = useRef(false);

    const fetchStatus = async () => {
        try {
            const response = await axios.get('/api/v3/media/scan/status');
            const newStatus = response.data;

            // Trigger library refresh if new games were added during this poll
            if (newStatus.isScanning && newStatus.gamesAddedCount > status.gamesAddedCount) {
                console.log(`ScannerStatus: Detected ${newStatus.gamesAddedCount - status.gamesAddedCount} new games. Triggering refresh...`);
                window.dispatchEvent(new Event('LIBRARY_UPDATED_EVENT'));
            }

            // If it just finished scanning
            if (prevIsScanning.current && !newStatus.isScanning) {
                console.log("ScannerStatus: Scan finished detected via polling");
                setShowFinished(true);
                // Auto-hide after 10 seconds (longer for finished message)
                setTimeout(() => setShowFinished(false), 10000);
            }

            // If it just started scanning
            if (!prevIsScanning.current && newStatus.isScanning) {
                console.log("ScannerStatus: Scan started detected via polling");
                setShowFinished(false);
            }

            setStatus(newStatus);
            prevIsScanning.current = newStatus.isScanning;
        } catch (error) {
            console.error("Error polling scanner status:", error);
        }
    };

    useEffect(() => {
        // Poll every 3 seconds
        const interval = setInterval(fetchStatus, 3000);
        fetchStatus(); // Initial fetch

        return () => clearInterval(interval);
    }, []);

    const handleBannerClick = async () => {
        if (showFinished) {
            console.log("ScannerStatus: Refreshing library via banner click");
            window.dispatchEvent(new Event('LIBRARY_UPDATED_EVENT'));
            setShowFinished(false);
        } else if (status.isScanning) {
            if (window.confirm("¿Deseas detener el escaneo actual?")) {
                try {
                    await axios.post('/api/v3/media/scan/stop');
                    console.log("ScannerStatus: Scan stop requested");
                } catch (error) {
                    console.error("Error stopping scan:", error);
                }
            }
        }
    };

    // If it's not scanning and we are not showing the finished message, don't render
    if (!status.isScanning && !showFinished) return null;

    return (
        <div className={`scanner-status ${showFinished ? 'finished' : 'scanning'}`}
            style={{ cursor: 'pointer' }}
            onClick={handleBannerClick}
            title={status.isScanning ? "Haz clic para detener el escaneo" : "Haz clic para refrescar la lista"}
        >
            <div className="scanner-status-content">
                {status.isScanning ? (
                    <>
                        <div className="scanner-spinner"></div>
                        <div className="scanner-text">
                            <span className="status-label">Escaneando biblioteca...</span>
                            {status.lastGameFound && (
                                <span className="game-label">Ultimo: <strong>{status.lastGameFound}</strong></span>
                            )}
                            <span className="count-label">({status.gamesAddedCount} nuevos juegos)</span>
                            <span className="status-hint">Haz click para cancelar</span>
                        </div>
                    </>
                ) : (
                    <div className="scanner-text">
                        <span className="status-label">✅ Escaneo completo</span>
                        <span className="count-label">Se añadieron {status.gamesAddedCount} juegos</span>
                        <span className="status-hint">Haz click para actualizar lista</span>
                    </div>
                )}
            </div>
        </div>
    );
};

export default ScannerStatus;
