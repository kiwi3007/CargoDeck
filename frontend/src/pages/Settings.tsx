import React, { useState, useEffect } from 'react';
import axios from 'axios';
import { t as translate, Language, getLanguage as getSavedLanguage, setLanguage as setGlobalLanguage } from '../i18n/translations';
import './Settings.css';
import igdbLogo from '../assets/igdb_logo.png';
import prowlarrLogo from '../assets/prowlarr_logo.png';
import languageIcon from '../assets/language_icon.png';
import downloadIcon from '../assets/download_icon.png';
import mediaFolderIcon from '../assets/media_folder_icon.png';
import jackettLogo from '../assets/jackett_logo.png';
import steamLogo from '../assets/steam_logo.png';
import pcIcon from '../assets/pc_icon.png';
import FolderExplorerModal from '../components/FolderExplorerModal';

interface DownloadClient {
  id?: number;
  name: string;
  implementation: string;
  host: string;
  port: number;
  username?: string;
  password?: string;
  category?: string;
  urlBase?: string;
  enable: boolean;
  priority: number;
}

const Settings: React.FC = () => {
  const [prowlarrUrl, setProwlarrUrl] = useState('http://localhost:9696');
  const [prowlarrApiKey, setProwlarrApiKey] = useState('');
  const [prowlarrTesting, setProwlarrTesting] = useState(false);
  const [prowlarrTestResult, setProwlarrTestResult] = useState<{ success: boolean; message: string } | null>(null);

  const [jackettUrl, setJackettUrl] = useState('http://localhost:9117');
  const [jackettApiKey, setJackettApiKey] = useState('');
  const [jackettTesting, setJackettTesting] = useState(false);
  const [jackettTestResult, setJackettTestResult] = useState<{ success: boolean; message: string } | null>(null);

  const [igdbClientId, setIgdbClientId] = useState('');
  const [igdbClientSecret, setIgdbClientSecret] = useState('');
  const [steamApiKey, setSteamApiKey] = useState('');
  const [steamId, setSteamId] = useState('');
  const [steamTesting, setSteamTesting] = useState(false);
  const [steamTestResult, setSteamTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [steamSyncing, setSteamSyncing] = useState(false);
  const [steamSyncResult, setSteamSyncResult] = useState<{ success: boolean; message: string } | null>(null);
  const [folderPath, setFolderPath] = useState('');
  const [scanning, setScanning] = useState(false);
  const [language, setLanguage] = useState<Language>(getSavedLanguage());
  const [forceUpdateCounter, setForceUpdateCounter] = useState(0); // Force re-render trigger
  const [showFolderExplorer, setShowFolderExplorer] = useState(false);

  const t = (key: any) => translate(key, language);

  const handleSaveLanguage = () => {
    setGlobalLanguage(language);
    alert(t('languageSaved'));
    // Optional: reload or trigger global state update if needed
  };

  const [downloadClients, setDownloadClients] = useState<DownloadClient[]>([]);
  const [showClientModal, setShowClientModal] = useState(false);
  const [editingClient, setEditingClient] = useState<DownloadClient | null>(null);
  const [clientForm, setClientForm] = useState<DownloadClient>({
    name: '',
    implementation: 'qBittorrent',
    host: 'localhost',
    port: 8080,
    username: 'admin',
    password: '',
    category: 'playerr',
    urlBase: '',
    enable: true,
    priority: 1
  });
  const [clientTesting, setClientTesting] = useState(false);
  const [clientTestResult, setClientTestResult] = useState<{ success: boolean; message: string; version?: string } | null>(null);

  useEffect(() => {
    // Load saved language
    const savedLang = localStorage.getItem('playerr_language');
    if (savedLang) setLanguage(savedLang as Language);
    loadDownloadClients();
    loadSettings();

    const handleFolderSelected = (event: Event) => {
      const customEvent = event as CustomEvent;
      const newPath = customEvent.detail;
      setFolderPath(newPath);
    };

    const handleSettingsUpdated = () => {
      // Reload settings from backend
      loadSettings();
      // User requested immediate scan, so we can trigger it here if needed,
      // but let's first ensure the text field updates (which loadSettings will do).
      // Since the backend already saved the path, we might want to ask the user to confirm scan
      // OR we can trigger it. The user said: "when pressing the button... the scan scans"
      // Let's trigger it automatically after a short delay to ensure state is settled.
      setTimeout(() => {
        // Optional: trigger scan automatically?
        // handleScanNow(); 
        // Be careful with recursion or loops.
        // For now, let's just alert/notify or rely on the user seeing the path updated.
      }, 500);
    };

    window.addEventListener('FOLDER_SELECTED_EVENT', handleFolderSelected);
    window.addEventListener('SETTINGS_UPDATED_EVENT', handleSettingsUpdated);

    return () => {
      window.removeEventListener('FOLDER_SELECTED_EVENT', handleFolderSelected);
      window.removeEventListener('SETTINGS_UPDATED_EVENT', handleSettingsUpdated);
    };
  }, []);

  const loadSettings = async () => {
    try {
      const prowlarrResponse = await axios.get('/api/v3/settings/prowlarr');
      setProwlarrUrl(prowlarrResponse.data.url);
      setProwlarrApiKey(prowlarrResponse.data.apiKey);

      const igdbResponse = await axios.get('/api/v3/settings/igdb');
      setIgdbClientId(igdbResponse.data.clientId);
      setIgdbClientSecret(igdbResponse.data.clientSecret);

      const mediaResponse = await axios.get('/api/v3/media');
      setFolderPath(mediaResponse.data.folderPath);

      const jackettResponse = await axios.get('/api/v3/settings/jackett');
      setJackettUrl(jackettResponse.data.url);
      setJackettApiKey(jackettResponse.data.apiKey);

      const steamResponse = await axios.get('/api/v3/settings/steam');
      setSteamApiKey(steamResponse.data.apiKey);
      setSteamId(steamResponse.data.steamId);
    } catch (error) {
      console.error('Error loading settings:', error);
    }
  };

  const loadDownloadClients = async () => {
    try {
      const response = await axios.get('/api/v3/downloadclient');
      setDownloadClients(response.data);
    } catch (error) {
      console.error('Error loading download clients:', error);
    }
  };

  const handleSaveMetadata = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await axios.post('/api/v3/metadata/igdb', {
        clientId: igdbClientId,
        clientSecret: igdbClientSecret,
      });
      alert(t('igdbSettingsSaved'));
    } catch (error: any) {
      console.error('Error saving IGDB settings:', error);
      alert(`${t('error')} ${t('saveMetadata')}: ${error.response?.data?.error || error.message}`);
    }
  };

  const handleSaveSteam = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await axios.post('/api/v3/settings/steam', {
        apiKey: steamApiKey,
        steamId: steamId
      });
      alert(t('steamSettingsSaved'));
    } catch (error: any) {
      console.error('Error saving Steam settings:', error);
      alert(`${t('error')} ${t('saveSteam')}: ${error.response?.data?.error || error.message}`);
    }
  };

  const handleDisconnectIgdb = async () => {
    if (!window.confirm(t('disconnectConfirm'))) return;

    try {
      await axios.delete('/api/v3/settings/igdb');
      setIgdbClientId('');
      setIgdbClientSecret('');
      alert(t('igdbSettingsSaved')); // Or a custom message
    } catch (error: any) {
      console.error('Error disconnecting IGDB:', error);
      alert(`${t('error')}: ${error.response?.data?.error || error.message}`);
    }
  };

  const handleDisconnectSteam = async () => {
    if (!window.confirm(t('disconnectConfirm'))) return;

    try {
      await axios.delete('/api/v3/settings/steam');
      setSteamApiKey('');
      setSteamId('');
      alert(t('steamSettingsSaved')); // Reusing "Saved" message or similar success
    } catch (error: any) {
      console.error('Error disconnecting Steam:', error);
      alert(`${t('error')}: ${error.response?.data?.error || error.message}`);
    }
  };

  // ... (existing handlers)


  const handleTestSteam = async () => {
    setSteamTesting(true);
    setSteamTestResult(null);

    try {
      const response = await axios.post('/api/v3/settings/steam/test', {
        apiKey: steamApiKey,
        steamId: steamId
      });

      setSteamTestResult({
        success: response.data.success,
        message: response.data.message
      });
    } catch (error: any) {
      setSteamTestResult({
        success: false,
        message: `✗ ${t('error')}: ${error.response?.data?.message || error.message}`
      });
    } finally {
      setSteamTesting(false);
    }
  };

  const handleSyncSteam = async () => {
    setSteamSyncing(true);
    setSteamSyncResult(null);

    try {
      const response = await axios.post('/api/v3/settings/steam/sync');

      setSteamSyncResult({
        success: response.data.success,
        message: response.data.message
      });
    } catch (error: any) {
      setSteamSyncResult({
        success: false,
        message: `✗ ${t('error')}: ${error.response?.data?.message || error.message}`
      });
    } finally {
      setSteamSyncing(false);
    }
  };

  const handleSaveMediaSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      // Use PascalCase to ensure backend binding works (if case-sensitive)
      await axios.post('/api/v3/media', { FolderPath: folderPath, Platform: 'default' });
      alert(t('mediaSettingsSaved'));
    } catch (error: any) {
      console.error('Error saving media settings:', error);
      alert(`${t('error')} ${t('mediaSettingsSaved')}: ${error.response?.data?.error || error.message}`);
    }
  };

  const handleScanNow = async () => {
    setScanning(true);
    try {
      await axios.post('/api/v3/media/scan', {
        folderPath: folderPath,
        platform: 'default'
      });
    } catch (error: any) {
      console.error('Error scanning media:', error);
      if (error.response?.status === 400) {
        alert(t('igdbRequired'));
      } else {
        alert(`${t('error')} ${t('scanNow')}: ${error.response?.data?.error || error.message}`);
      }
    } finally {
      setScanning(false);
    }
  };

  const handleSaveProwlarr = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await axios.post('/api/v3/settings/prowlarr', {
        url: prowlarrUrl,
        apiKey: prowlarrApiKey
      });
      alert(t('prowlarrSettingsSaved'));
    } catch (error: any) {
      console.error('Error saving Prowlarr settings:', error);
      alert(`${t('error')} ${t('saveProwlarr')}: ${error.response?.data?.error || error.message}`);
    }
  };

  const handleTestProwlarr = async () => {
    setProwlarrTesting(true);
    setProwlarrTestResult(null);

    try {
      const response = await axios.post('/api/v3/search/test', {
        url: prowlarrUrl,
        apiKey: prowlarrApiKey
      });

      setProwlarrTestResult({
        success: response.data.connected,
        message: response.data.connected ? t('connectionSuccessful') : t('connectionFailed')
      });
    } catch (error: any) {
      setProwlarrTestResult({
        success: false,
        message: `✗ ${t('error')}: ${error.response?.data?.message || error.message}`
      });
    } finally {
      setProwlarrTesting(false);
    }
  };

  const handleSaveJackett = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await axios.post('/api/v3/settings/jackett', {
        url: jackettUrl,
        apiKey: jackettApiKey
      });
      alert(t('jackettSettingsSaved'));
    } catch (error: any) {
      console.error('Error saving Jackett settings:', error);
      alert(`${t('error')} ${t('saveJackett')}: ${error.response?.data?.error || error.message}`);
    }
  };

  const handleTestJackett = async () => {
    setJackettTesting(true);
    setJackettTestResult(null);

    try {
      const response = await axios.post('/api/v3/search/test', {
        url: jackettUrl,
        apiKey: jackettApiKey,
        type: 'jackett'
      });

      setJackettTestResult({
        success: response.data.connected,
        message: response.data.connected ? t('connectionSuccessful') : t('connectionFailed')
      });
    } catch (error: any) {
      setJackettTestResult({
        success: false,
        message: `✗ ${t('error')}: ${error.response?.data?.message || error.message}`
      });
    } finally {
      setJackettTesting(false);
    }
  };

  const handleTestDownloadClient = async () => {
    setClientTesting(true);
    setClientTestResult(null);

    try {
      const response = await axios.post('/api/v3/downloadclient/test', {
        implementation: clientForm.implementation,
        host: clientForm.host,
        port: clientForm.port,
        username: clientForm.username,
        password: clientForm.password,
        urlBase: clientForm.urlBase
      });

      setClientTestResult({
        success: response.data.connected,
        message: response.data.message,
        version: response.data.version
      });
    } catch (error: any) {
      setClientTestResult({
        success: false,
        message: `${t('error')}: ${error.response?.data?.message || error.message}`
      });
    } finally {
      setClientTesting(false);
    }
  };

  const handleSaveDownloadClient = async (e: React.FormEvent) => {
    e.preventDefault();

    try {
      if (editingClient?.id) {
        await axios.put(`/api/v3/downloadclient/${editingClient.id}`, clientForm);
      } else {
        await axios.post('/api/v3/downloadclient', clientForm);
      }

      await loadDownloadClients();
      setShowClientModal(false);
      resetClientForm();
    } catch (error) {
      alert(t('failedToSaveClient'));
    }
  };

  const handleDeleteClient = async (id: number) => {
    // Removed confirmation as per user request to avoid UI issues
    // if (!confirm(t('confirmDeleteClient'))) {
    //   return;
    // }

    try {
      await axios.delete(`/api/v3/downloadclient/${id}`);
      await loadDownloadClients();
    } catch (error) {
      console.error('Error deleting download client:', error);
      alert(t('failedToDeleteClient'));
    }
  };

  const resetClientForm = () => {
    setClientForm({
      name: '',
      implementation: 'qBittorrent',
      host: 'localhost',
      port: 8080,
      username: 'admin',
      password: '',
      category: 'playerr',
      urlBase: '',
      enable: true,
      priority: 1
    });
    setEditingClient(null);
    setClientTestResult(null);
  };

  const openAddClientModal = () => {
    resetClientForm();
    setShowClientModal(true);
  };

  const openEditClientModal = (client: DownloadClient) => {
    setEditingClient(client);
    setClientForm({ ...client });
    setShowClientModal(true);
  };

  // Polling mechanism for folder selection
  // This is a robust fallback in case the native message event is lost or delayed
  const startFolderPolling = () => {
    setScanning(true); // Re-use scanning state or simple UI indication if preferred
    let attempts = 0;
    const initialPath = folderPath;

    const pollInterval = setInterval(async () => {
      attempts++;
      try {
        // Add timestamp to prevent caching
        const response = await axios.get(`/api/v3/media?t=${Date.now()}`);
        const currentPath = response.data.folderPath;

        if (currentPath !== initialPath) {
          // Path changed! Update UI and stop polling
          setFolderPath(currentPath);
          setForceUpdateCounter(prev => prev + 1); // FORCE React to re-render
          clearInterval(pollInterval);
          setScanning(false);
        }
      } catch (e) {
        console.error("Polling error", e);
      }

      // Stop after 30 seconds (approx 60 attempts)
      if (attempts > 60) {
        clearInterval(pollInterval);
        setScanning(false);
      }
    }, 500);
  };

  return (
    <div className="settings">
      <div className="settings-section">
        <div className="section-header-with-logo">
          <h3>{t('mediaFolderTitle')}</h3>
        </div>
        <p className="settings-description">
          {t('mediaFolderDesc')}
        </p>
        <form onSubmit={handleSaveMediaSettings}>
          <div className="form-group">
            <label htmlFor="folder-path">{t('mediaFolderPath')}</label>
            <div style={{ display: 'flex', gap: '10px' }}>
              <input
                id="folder-path"
                type="text"
                value={folderPath}
                onChange={(e) => setFolderPath(e.target.value)}
                placeholder="/home/user/games"
                style={{ flex: 1 }}
              />
              <button
                type="button"
                className="btn-secondary"
                onClick={() => {
                  // @ts-ignore
                  if (window.external && window.external.sendMessage) {
                    // Start polling BEFORE we open the modal (or right as we do)
                    // Note: ShowOpenFolder in backend blocks, so polling might be blocked if JS thread is blocked?
                    // Photino usually runs native window on separate thread, but WebView interactions can be tricky.
                    // If JS is blocked, this polling will resume immediately after dialog closes, which is perfect.
                    startFolderPolling();

                    // @ts-ignore
                    window.external.sendMessage('SELECT_FOLDER');
                  } else {
                    setShowFolderExplorer(true);
                  }
                }}
                title={t('selectFolder')}
              >
                📂
              </button>
            </div>
          </div>

          <div className="button-group">
            <button type="submit" className="btn-primary">{t('save')}</button>
            <button
              type="button"
              className="btn-secondary"
              onClick={handleScanNow}
              disabled={scanning || !folderPath}
            >
              {scanning ? t('scanning') : t('scanNow')}
            </button>
          </div>
        </form>
      </div>

      <div className="settings-section">
        <div className="section-header-with-logo">
          <img src={steamLogo} alt="Steam" className="steam-logo" style={{ height: '60px' }} />
        </div>
        <p className="settings-description">
          {t('steamDesc')}
        </p>
        <form onSubmit={handleSaveSteam}>
          <div className="form-group">
            <label htmlFor="steam-api-key">{t('steamApiKey')}</label>
            <input
              type="password"
              id="steam-api-key"
              placeholder={t('steamApiKey')}
              value={steamApiKey}
              onChange={(e) => setSteamApiKey(e.target.value)}
            />
            <small>{t('steamApiKeyHelp')} <a href="https://steamcommunity.com/dev/apikey" target="_blank" rel="noopener noreferrer">{t('steamDevPage')}</a></small>
          </div>
          <div className="form-group">
            <label htmlFor="steam-id">{t('steamId')}</label>
            <input
              type="text"
              id="steam-id"
              placeholder={t('steamId')}
              value={steamId}
              onChange={(e) => setSteamId(e.target.value)}
            />
          </div>
          <div className="button-group">
            <button
              type="button"
              className="btn-secondary"
              onClick={handleTestSteam}
              disabled={steamTesting || !steamApiKey || !steamId}
            >
              {steamTesting ? t('testing') : t('testConnection')}
            </button>
            <button
              type="button"
              className="btn-secondary"
              onClick={handleSyncSteam}
              disabled={steamSyncing || !steamApiKey || !steamId}
            >
              {steamSyncing ? t('syncing') : t('syncLibrary')}
            </button>
            <button type="submit" className="btn-primary">{t('saveSteam')}</button>
            {steamApiKey && (
              <button
                type="button"
                className="btn-delete"
                onClick={handleDisconnectSteam}
                style={{ marginLeft: '10px' }}
              >
                {t('disconnect')}
              </button>
            )}
          </div>

          {steamTestResult && (
            <div className={`test-result ${steamTestResult.success ? 'success' : 'error'}`}>
              {steamTestResult.message}
            </div>
          )}

          {steamSyncResult && (
            <div className={`test-result ${steamSyncResult.success ? 'success' : 'error'}`}>
              {steamSyncResult.message}
            </div>
          )}
        </form>
      </div>

      <div className="settings-section">
        <div className="section-header-with-logo">
          <img src={igdbLogo} alt="IGDB" className="igdb-logo" />
        </div>
        <p className="settings-description">
          {t('metadataDesc')}
        </p>
        <form onSubmit={handleSaveMetadata}>
          <div className="form-group">
            <label htmlFor="igdb-client-id">{t('igdbClientId')}</label>
            <input
              type="text"
              id="igdb-client-id"
              placeholder={t('igdbClientId')}
              value={igdbClientId}
              onChange={(e) => setIgdbClientId(e.target.value)}
            />
            <small>{t('twitchCredentialsHelp')} <a href="https://dev.twitch.tv/console/apps" target="_blank" rel="noopener noreferrer">{t('twitchConsole')}</a></small>
          </div>
          <div className="form-group">
            <label htmlFor="igdb-client-secret">{t('igdbClientSecret')}</label>
            <input
              type="password"
              id="igdb-client-secret"
              placeholder={t('igdbClientSecret')}
              value={igdbClientSecret}
              onChange={(e) => setIgdbClientSecret(e.target.value)}
            />
          </div>
          <div className="button-group">
            <button type="submit" className="btn-primary">{t('saveMetadata')}</button>
            {igdbClientId && (
              <button
                type="button"
                className="btn-delete"
                onClick={handleDisconnectIgdb}
                style={{ marginLeft: '10px' }}
              >
                {t('disconnect')}
              </button>
            )}
          </div>
        </form>
      </div>

      <div className="settings-section">
        <div className="section-header-with-logo">
          <img src={languageIcon} alt="Language" className="language-icon" />
        </div>
        <p className="settings-description">
          {t('languageDesc')}
        </p>
        <div className="form-group">
          <label htmlFor="language-select">{t('languageTitle')}</label>
          <select
            id="language-select"
            value={language}
            onChange={(e) => setLanguage(e.target.value as Language)}
          >
            <option value="es">Español</option>
            <option value="en">English</option>
            <option value="fr">Français</option>
            <option value="de">Deutsch</option>
            <option value="ru">Русский</option>
            <option value="zh">中文</option>
            <option value="ja">日本語</option>
          </select>
        </div>
        <button type="button" className="btn-primary" onClick={handleSaveLanguage}>{t('saveLanguage')}</button>
      </div>

      <div className="settings-section">
        <div className="section-header-with-logo">
          <img src={prowlarrLogo} alt="Prowlarr" className="prowlarr-logo" />
        </div>
        <p className="settings-description">
          {t('prowlarrDesc')}
        </p>
        <form onSubmit={handleSaveProwlarr}>
          <div className="form-group">
            <label htmlFor="prowlarr-url">{t('prowlarrUrl')}</label>
            <input
              type="text"
              id="prowlarr-url"
              placeholder={t('prowlarrUrlPlaceholder')}
              value={prowlarrUrl}
              onChange={(e) => setProwlarrUrl(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label htmlFor="prowlarr-api">{t('prowlarrApiKey')}</label>
            <input
              type="password"
              id="prowlarr-api"
              placeholder={t('prowlarrApiKeyPlaceholder')}
              value={prowlarrApiKey}
              onChange={(e) => setProwlarrApiKey(e.target.value)}
              required
            />
          </div>

          {prowlarrTestResult && (
            <div className={`test-result ${prowlarrTestResult.success ? 'success' : 'error'}`}>
              {prowlarrTestResult.message}
            </div>
          )}

          <div className="button-group">
            <button
              type="button"
              className="btn-secondary"
              onClick={handleTestProwlarr}
              disabled={prowlarrTesting || !prowlarrUrl || !prowlarrApiKey}
            >
              {prowlarrTesting ? t('testing') : t('testConnection')}
            </button>
            <button type="submit" className="btn-primary">{t('saveProwlarr')}</button>
          </div>
        </form>
      </div>

      <div className="settings-section">
        <div className="section-header-with-logo">
          <img src={jackettLogo} alt="Jackett" className="jackett-logo" />
        </div>
        <p className="settings-description">
          {t('jackettDesc')}
        </p>
        <form onSubmit={handleSaveJackett}>
          <div className="form-group">
            <label htmlFor="jackett-url">{t('jackettUrl')}</label>
            <input
              type="text"
              id="jackett-url"
              placeholder={t('jackettUrlPlaceholder')}
              value={jackettUrl}
              onChange={(e) => setJackettUrl(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label htmlFor="jackett-api">{t('jackettApiKey')}</label>
            <input
              type="password"
              id="jackett-api"
              placeholder={t('jackettApiKeyPlaceholder')}
              value={jackettApiKey}
              onChange={(e) => setJackettApiKey(e.target.value)}
              required
            />
          </div>

          {jackettTestResult && (
            <div className={`test-result ${jackettTestResult.success ? 'success' : 'error'}`}>
              {jackettTestResult.message}
            </div>
          )}

          <div className="button-group">
            <button
              type="button"
              className="btn-secondary"
              onClick={handleTestJackett}
              disabled={jackettTesting || !jackettUrl || !jackettApiKey}
            >
              {jackettTesting ? t('testing') : t('testConnection')}
            </button>
            <button type="submit" className="btn-primary">{t('saveJackett')}</button>
          </div>
        </form>
      </div>

      <div className="settings-section">
        <div className="section-header-with-logo">
          <img src={downloadIcon} alt="Downloads" className="download-icon" />
        </div>
        <p className="settings-description">
          {t('downloadClientsDesc')}
        </p>

        {downloadClients.length > 0 && (
          <div className="clients-list">
            {downloadClients.map(client => (
              <div key={client.id} className={`client-card ${!client.enable ? 'disabled' : ''}`}>
                <div className="client-info">
                  <h4>{client.name}</h4>
                  <p>{client.implementation} - {client.host}:{client.port}</p>
                  {client.category && <span className="category-badge">{client.category}</span>}
                </div>
                <div className="client-actions">
                  <button
                    className="btn-edit"
                    onClick={() => openEditClientModal(client)}
                  >
                    {t('edit')}
                  </button>
                  <button
                    className="btn-delete"
                    onClick={() => handleDeleteClient(client.id!)}
                  >
                    {t('delete')}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        <button className="btn-secondary" onClick={openAddClientModal}>
          {t('addClientButton')}
        </button>
      </div>

      {showClientModal && (
        <div className="modal-overlay" onClick={() => setShowClientModal(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>{editingClient ? t('editDownloadClient') : t('addDownloadClient')}</h3>
              <button className="modal-close" onClick={() => setShowClientModal(false)}>×</button>
            </div>

            <form onSubmit={handleSaveDownloadClient}>
              <div className="form-group">
                <label>{t('name')}</label>
                <input
                  type="text"
                  value={clientForm.name}
                  onChange={(e) => setClientForm({ ...clientForm, name: e.target.value })}
                  placeholder={t('clientNamePlaceholder')}
                  required
                />
              </div>

              <div className="form-group">
                <label>{t('implementation')}</label>
                <select
                  value={clientForm.implementation}
                  onChange={(e) => setClientForm({ ...clientForm, implementation: e.target.value })}
                  required
                >
                  <option value="qBittorrent">qBittorrent</option>
                  <option value="Transmission">Transmission</option>
                  <option value="Deluge">Deluge ({t('comingSoon')})</option>
                </select>
              </div>

              <div className="form-row">
                <div className="form-group">
                  <label>{t('host')}</label>
                  <input
                    type="text"
                    value={clientForm.host}
                    onChange={(e) => setClientForm({ ...clientForm, host: e.target.value })}
                    placeholder={t('hostPlaceholder')}
                    required
                  />
                </div>

                <div className="form-group">
                  <label>{t('port')}</label>
                  <input
                    type="number"
                    value={clientForm.port}
                    onChange={(e) => setClientForm({ ...clientForm, port: parseInt(e.target.value) })}
                    placeholder={t('portPlaceholder')}
                    required
                  />
                </div>
              </div>

              <div className="form-group">
                <label>{t('username')}</label>
                <input
                  type="text"
                  value={clientForm.username || ''}
                  onChange={(e) => setClientForm({ ...clientForm, username: e.target.value })}
                  placeholder={t('usernamePlaceholder')}
                />
              </div>

              <div className="form-group">
                <label>{t('password')}</label>
                <input
                  type="password"
                  value={clientForm.password || ''}
                  onChange={(e) => setClientForm({ ...clientForm, password: e.target.value })}
                  placeholder={t('passwordPlaceholder')}
                />
              </div>

              <div className="form-group">
                <label>{t('urlBase')} <small>(optional, e.g. /qbittorrent)</small></label>
                <input
                  type="text"
                  value={clientForm.urlBase || ''}
                  onChange={(e) => setClientForm({ ...clientForm, urlBase: e.target.value })}
                  placeholder="/qbittorrent"
                />
              </div>

              <div className="form-group">
                <label>{t('category')}</label>
                <input
                  type="text"
                  value={clientForm.category || ''}
                  onChange={(e) => setClientForm({ ...clientForm, category: e.target.value })}
                  placeholder={t('categoryPlaceholder')}
                />
                <small>{t('torrentsCategoryHint')}</small>
              </div>

              <div className="form-group checkbox-group">
                <label>
                  <input
                    type="checkbox"
                    checked={clientForm.enable}
                    onChange={(e) => setClientForm({ ...clientForm, enable: e.target.checked })}
                  />
                  {t('enableThisClient')}
                </label>
              </div>

              {clientTestResult && (
                <div className={`test-result ${clientTestResult.success ? 'success' : 'error'}`}>
                  {clientTestResult.message}
                  {clientTestResult.version && <div>{t('versionHeader')}: {clientTestResult.version}</div>}
                </div>
              )}
              <div className="modal-actions">
                <button
                  type="button"
                  className="btn-secondary"
                  onClick={handleTestDownloadClient}
                  disabled={clientTesting}
                >
                  {clientTesting ? t('testing') : t('testConnection')}
                </button>
                <button type="submit" className="btn-primary">
                  {editingClient ? t('updateClient') : t('addClientButton')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
      {showFolderExplorer && (
        <FolderExplorerModal
          initialPath={folderPath}
          onSelect={(path) => {
            setFolderPath(path);
            setShowFolderExplorer(false);
          }}
          onClose={() => setShowFolderExplorer(false)}
          language={language}
        />
      )}
    </div>
  );
};

export default Settings;
