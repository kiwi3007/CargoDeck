import React, { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import axios from 'axios';
import { t as translate, Language, getLanguage as getSavedLanguage, setLanguage as setGlobalLanguage } from '../i18n/translations';
import './Settings.css';
import igdbLogo from '../assets/igdb_logo.png';
import prowlarrLogo from '../assets/prowlarr_logo.png';
import languageIcon from '../assets/language_icon.png';
import mediaFolderIcon from '../assets/media_folder_icon.png';
import jackettLogo from '../assets/jackett_logo.png';
import steamLogo from '../assets/steam_logo.png';
import pcIcon from '../assets/pc_icon.png';
import torrentNzbIcon from '../assets/TORRENT_NZB_icon.png';
import FolderExplorerModal from '../components/FolderExplorerModal';
import HydraSourceModal from '../components/HydraSourceModal';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faFolderOpen, faPlus, faEdit, faTrash, faCheckCircle, faTimesCircle, faPlay, faStop, faSync, faTimes } from '@fortawesome/free-solid-svg-icons';

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
  apiKey?: string;
  enable: boolean;
  useSsl?: boolean;
  priority: number;
  // Remote Path Mapping
  remotePathMapping?: string;
  localPathMapping?: string;
}

interface HydraConfiguration {
  id: number;
  name: string;
  url: string;
  enabled: boolean;
}

const Settings: React.FC = () => {
  const location = useLocation();
  const currentTab = location.hash.replace('#', '') || 'media';
  const [prowlarrUrl, setProwlarrUrl] = useState('');
  const [prowlarrApiKey, setProwlarrApiKey] = useState('');
  const [prowlarrEnabled, setProwlarrEnabled] = useState(true);
  const [showProwlarrModal, setShowProwlarrModal] = useState(false);
  const [prowlarrTesting, setProwlarrTesting] = useState(false);
  const [prowlarrTestResult, setProwlarrTestResult] = useState<{ success: boolean; message: string } | null>(null);

  const [jackettUrl, setJackettUrl] = useState('');
  const [jackettApiKey, setJackettApiKey] = useState('');
  const [jackettEnabled, setJackettEnabled] = useState(true);
  const [showJackettModal, setShowJackettModal] = useState(false);
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
  const [downloadPath, setDownloadPath] = useState('');
  const [destinationPath, setDestinationPath] = useState('');
  const [winePrefixPath, setWinePrefixPath] = useState('');
  const [scanning, setScanning] = useState(false);
  const [activeFolderField, setActiveFolderField] = useState<'media' | 'download' | 'destination' | 'wine' | 'clientLocalPath'>('media');
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
    apiKey: '',
    enable: true,
    useSsl: false,
    priority: 1
  });
  const [clientTesting, setClientTesting] = useState(false);
  const [clientTestResult, setClientTestResult] = useState<{ success: boolean; message: string; version?: string } | null>(null);

  const [postDownloadSettings, setPostDownloadSettings] = useState({
    enableAutoMove: true,
    enableAutoExtract: true,
    enableDeepClean: true,
    enableAutoRename: true,
    monitorIntervalSeconds: 60,
    unwantedExtensions: ['.txt', '.nfo', '.url']
  });

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

  const [hydraSources, setHydraSources] = useState<HydraConfiguration[]>([]);
  const [showHydraModal, setShowHydraModal] = useState(false);
  const [editingHydraSource, setEditingHydraSource] = useState<HydraConfiguration | null>(null);
  const [deleteConfirmation, setDeleteConfirmation] = useState<{ id: number; name: string } | null>(null);

  useEffect(() => {
    loadHydraSources();
  }, []);

  const loadHydraSources = async () => {
    try {
      const response = await axios.get('/api/v3/hydra');
      if (Array.isArray(response.data)) {
        setHydraSources(response.data);
      } else {
        console.warn('Hydra API returned non-array:', response.data);
        setHydraSources([]);
      }
    } catch (error) {
      console.error('Error loading Hydra sources:', error);
      setHydraSources([]);
    }
  };

  const handleOpenAddHydra = () => {
    setEditingHydraSource(null);
    setShowHydraModal(true);
  };

  const handleOpenEditHydra = (source: HydraConfiguration) => {
    setEditingHydraSource(source);
    setShowHydraModal(true);
  };

  const handleDeleteHydra = (source: HydraConfiguration) => {
    setDeleteConfirmation({ id: source.id, name: source.name });
  };

  const confirmDeleteHydra = async () => {
    if (!deleteConfirmation) return;

    try {
      await axios.delete(`/api/v3/hydra/${deleteConfirmation.id}`);
      loadHydraSources();
      setDeleteConfirmation(null);
    } catch (error) {
      alert('Error deleting source');
    }
  };

  const loadSettings = async () => {
    try {
      const prowlarrResponse = await axios.get('/api/v3/settings/prowlarr');
      setProwlarrUrl(prowlarrResponse.data.url);
      setProwlarrApiKey(prowlarrResponse.data.apiKey);
      setProwlarrEnabled(prowlarrResponse.data.enabled !== false); // Default true if missing

      const igdbResponse = await axios.get('/api/v3/settings/igdb');
      setIgdbClientId(igdbResponse.data.clientId);
      setIgdbClientSecret(igdbResponse.data.clientSecret);

      const mediaResponse = await axios.get('/api/v3/media');
      setFolderPath(mediaResponse.data.folderPath);
      setDownloadPath(mediaResponse.data.downloadPath || '');
      setDestinationPath(mediaResponse.data.destinationPath || '');
      setWinePrefixPath(mediaResponse.data.winePrefixPath || '');

      const jackettResponse = await axios.get('/api/v3/settings/jackett');
      setJackettUrl(jackettResponse.data.url);
      setJackettApiKey(jackettResponse.data.apiKey);
      setJackettEnabled(jackettResponse.data.enabled !== false); // Default true

      const steamResponse = await axios.get('/api/v3/settings/steam');
      setSteamApiKey(steamResponse.data.apiKey);
      setSteamId(steamResponse.data.steamId);

      const postDownloadResponse = await axios.get('/api/v3/postdownload');
      setPostDownloadSettings(postDownloadResponse.data);
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

  const saveMediaConfig = async (overrides?: { folderPath?: string, downloadPath?: string, destinationPath?: string, winePrefixPath?: string }) => {
    try {
      // Use PascalCase to ensure backend binding works (if case-sensitive)
      await axios.post('/api/v3/media', {
        FolderPath: overrides?.folderPath ?? folderPath,
        DownloadPath: overrides?.downloadPath ?? downloadPath,
        DestinationPath: overrides?.destinationPath ?? destinationPath,
        WinePrefixPath: overrides?.winePrefixPath ?? winePrefixPath,
        Platform: 'default'
      });
      console.log('Media settings saved successfully');
    } catch (error: any) {
      console.error('Error saving media settings:', error);
      alert(`${t('error')} ${t('mediaSettingsSaved')}: ${error.response?.data?.error || error.message}`);
    }
  };

  const handleSaveMediaSettings = (e: React.FormEvent) => {
    e.preventDefault();
    saveMediaConfig();
  };

  const handleScanNow = async (specificPath?: string) => {
    // 1. Set loading state immediately
    setScanning(true);

    try {
      // 2. Trigger start
      // If specificPath is provided (e.g. Wine), use it. Otherwise use main folderPath.
      await axios.post('/api/v3/media/scan', {
        folderPath: specificPath || folderPath,
        platform: 'default'
      });
      // Do NOT setScanning(false) here, relying on the poller below to detect when it finishes.
    } catch (error: any) {
      console.error('Error scanning media:', error);
      // Only turn off if we failed to START the scan
      setScanning(false);

      if (error.response?.status === 400) {
        alert(t('igdbRequired'));
      } else {
        alert(`${t('error')} ${t('scanNow')}: ${error.response?.data?.error || error.message}`);
      }
    }
  };

  // Monitor Scan Status
  useEffect(() => {
    let intervalId: any;

    if (scanning) {
      // Poll every 1 second
      intervalId = setInterval(async () => {
        try {
          const response = await axios.get('/api/v3/media/scan/status');
          const isScanning = response.data.isScanning;

          // If backend says it finished, update UI
          if (!isScanning) {
            setScanning(false);
          }
        } catch (err) {
          console.error("Error polling scan status:", err);
          // If poll fails repeatedly, should we stop? For now, let's keep trying or stop on definitive error?
          // Stopping to avoid infinite error loops
          setScanning(false);
        }
      }, 1000);
    }

    return () => {
      if (intervalId) clearInterval(intervalId);
    };
  }, [scanning]);



  const toggleHydra = async (source: HydraConfiguration) => {
    const newState = !source.enabled;
    const updatedSources = hydraSources.map(s => s.id === source.id ? { ...s, enabled: newState } : s);
    setHydraSources(updatedSources); // Optimistic update

    try {
      await axios.put(`/api/v3/hydra/${source.id}`, {
        ...source,
        enabled: newState
      });
    } catch (error) {
      console.error('Error toggling Hydra source:', error);
      setHydraSources(hydraSources); // Revert
      alert('Failed to update source status');
    }
  };

  const toggleProwlarr = async () => {
    const newState = !prowlarrEnabled;
    setProwlarrEnabled(newState); // Optimistic

    try {
      await axios.post('/api/v3/settings/prowlarr', {
        url: prowlarrUrl,
        apiKey: prowlarrApiKey,
        enabled: newState
      });
    } catch (error) {
      console.error('Error toggling Prowlarr:', error);
      setProwlarrEnabled(!newState); // Revert
      alert('Failed to update Prowlarr status');
    }
  };

  const toggleJackett = async () => {
    const newState = !jackettEnabled;
    setJackettEnabled(newState); // Optimistic

    try {
      await axios.post('/api/v3/settings/jackett', {
        url: jackettUrl,
        apiKey: jackettApiKey,
        enabled: newState
      });
    } catch (error) {
      console.error('Error toggling Jackett:', error);
      setJackettEnabled(!newState); // Revert
      alert('Failed to update Jackett status');
    }
  };

  const handleSaveProwlarr = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await axios.post('/api/v3/settings/prowlarr', {
        url: prowlarrUrl,
        apiKey: prowlarrApiKey,
        enabled: prowlarrEnabled
      });
      alert(t('prowlarrSettingsSaved'));
      setShowProwlarrModal(false);
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
        apiKey: jackettApiKey,
        enabled: jackettEnabled
      });
      alert(t('jackettSettingsSaved'));
      setShowJackettModal(false);
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
        urlBase: clientForm.urlBase,
        apiKey: clientForm.apiKey
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

  const handleSaveDownloadClient = async (e: any) => {
    if (e && e.preventDefault) e.preventDefault();

    // alert('DEBUG: Attempting to save client...'); // Removed to reduce noise, but good for testing

    try {
      const payload = { ...clientForm };

      // Ensure numbers are numbers
      payload.port = parseInt(String(payload.port));
      payload.priority = parseInt(String(payload.priority));

      // alert(`DEBUG: Sending payload: ${JSON.stringify(payload)}`);

      if (editingClient?.id) {
        await axios.put(`/api/v3/downloadclient/${editingClient.id}`, payload);
      } else {
        await axios.post('/api/v3/downloadclient', payload);
      }

      await loadDownloadClients();
      setShowClientModal(false);
      resetClientForm();
      // alert('Client saved successfully!');
    } catch (error: any) {
      console.error('Error saving client:', error);
      alert(`${t('failedToSaveClient')}: ${error.response?.data?.message || error.message || 'Unknown error'}`);
    }
  };
  const toggleDownloadClient = async (client: DownloadClient) => {
    const newState = !client.enable;
    // Optimistic Update
    const updatedClients = downloadClients.map(c => c.id === client.id ? { ...c, enable: newState } : c);
    setDownloadClients(updatedClients);

    try {
      await axios.put(`/api/v3/downloadclient/${client.id}`, {
        ...client,
        enable: newState
      });
    } catch (error) {
      console.error('Error toggling download client:', error);
      // Revert
      const revertedClients = downloadClients.map(c => c.id === client.id ? { ...c, enable: !newState } : c);
      setDownloadClients(revertedClients);
      alert('Failed to update client status');
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
      apiKey: '',
      enable: true,
      useSsl: false,
      priority: 1
    });
    setEditingClient(null);
    setClientTestResult(null);
  };

  const handleSavePostDownload = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await axios.post('/api/v3/postdownload', postDownloadSettings);
      alert(t('postDownloadSettingsSaved'));
    } catch (error: any) {
      console.error('Error saving post-download settings:', error);
      alert(`${t('error')}: ${error.response?.data?.message || error.message}`);
    }
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
        const currentDownloadPath = response.data.downloadPath;
        const currentDestinationPath = response.data.destinationPath;

        if (currentPath !== initialPath || currentDownloadPath !== downloadPath || currentDestinationPath !== destinationPath) {
          // Path changed! Update UI and stop polling
          setFolderPath(currentPath);
          setDownloadPath(currentDownloadPath || '');
          setDestinationPath(currentDestinationPath || '');
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
      {currentTab === 'media' && (
        <>
          <div className="settings-section" id="media">
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
                    onBlur={() => saveMediaConfig({ folderPath: folderPath })}
                    placeholder="/home/user/games"
                    style={{ flex: 1 }}
                  />
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => handleScanNow()}
                    disabled={scanning || !folderPath}
                    title={t('scanNow')}
                  >
                    <FontAwesomeIcon icon={faSync} spin={scanning} />
                  </button>
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => {
                      // @ts-ignore
                      if (window.external && window.external.sendMessage) {
                        startFolderPolling();
                        // @ts-ignore
                        window.external.sendMessage('SELECT_FOLDER');
                      } else {
                        setActiveFolderField('media');
                        setShowFolderExplorer(true);
                      }
                    }}
                    title={t('selectFolder')}
                  >
                    <FontAwesomeIcon icon={faFolderOpen} />
                  </button>
                </div>
              </div>

              <div className="form-group">
                <label htmlFor="download-path">{t('downloadPath')}</label>
                <div style={{ display: 'flex', gap: '10px' }}>
                  <input
                    id="download-path"
                    type="text"
                    value={downloadPath}
                    onChange={(e) => setDownloadPath(e.target.value)}
                    onBlur={() => saveMediaConfig({ downloadPath: downloadPath })}
                    placeholder="/Volumes/Downloads"
                    style={{ flex: 1 }}
                  />
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => {
                      // @ts-ignore
                      if (window.external && window.external.sendMessage) {
                        startFolderPolling();
                        // @ts-ignore
                        window.external.sendMessage('SELECT_FOLDER:DOWNLOAD');
                      } else {
                        setActiveFolderField('download');
                        setShowFolderExplorer(true);
                      }
                    }}
                  >
                    <FontAwesomeIcon icon={faFolderOpen} />
                  </button>
                </div>
              </div>

              <div className="form-group">
                <label htmlFor="destination-path">{t('destinationPath')}</label>
                <div style={{ display: 'flex', gap: '10px' }}>
                  <input
                    id="destination-path"
                    type="text"
                    value={destinationPath}
                    onChange={(e) => setDestinationPath(e.target.value)}
                    onBlur={() => saveMediaConfig({ destinationPath: destinationPath })}
                    placeholder="/Volumes/Media/Games"
                    style={{ flex: 1 }}
                  />
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => {
                      // @ts-ignore
                      if (window.external && window.external.sendMessage) {
                        startFolderPolling();
                        // @ts-ignore
                        window.external.sendMessage('SELECT_FOLDER:DESTINATION');
                      } else {
                        setActiveFolderField('destination');
                        setShowFolderExplorer(true);
                      }
                    }}
                  >
                    <FontAwesomeIcon icon={faFolderOpen} />
                  </button>
                </div>
              </div>

              <div className="form-group" style={{ marginTop: '20px', borderTop: '1px solid #444', paddingTop: '15px' }}>
                <div className="section-header-with-logo">
                  <h4>{t('wineIntegration')}</h4>
                </div>
                <p className="settings-description-sm" style={{ fontSize: '0.85em', color: '#aaa' }}>{t('winePrefixPathDesc')}</p>

                <label htmlFor="wine-path">{t('winePrefixPath')}</label>
                <div style={{ display: 'flex', gap: '10px' }}>
                  <input
                    id="wine-path"
                    type="text"
                    value={winePrefixPath}
                    onChange={(e) => setWinePrefixPath(e.target.value)}
                    onBlur={() => saveMediaConfig({ winePrefixPath: winePrefixPath })}
                    placeholder="/Users/name/Library/Containers/com.isaacmarovitz.Whisky/Bottles/..."
                    style={{ flex: 1 }}
                  />
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => handleScanNow(winePrefixPath)}
                    disabled={scanning || !winePrefixPath}
                    title={t('scanNow')}
                  >
                    <FontAwesomeIcon icon={faSync} spin={scanning} />
                  </button>
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => {
                      setActiveFolderField('wine');
                      setShowFolderExplorer(true);
                    }}
                    title={t('selectFolder')}
                  >
                    <FontAwesomeIcon icon={faFolderOpen} />
                  </button>
                </div>
              </div>
            </form>
          </div>

          <div className="settings-section" id="post-download">
            <div className="section-header-with-logo">
              <h3>{t('postDownloadTitle')}</h3>
            </div>
            <p className="settings-description">
              {t('postDownloadDesc')}
            </p>
            <form onSubmit={handleSavePostDownload}>
              <div className="form-group checkbox-group">
                <label htmlFor="enable-auto-move">
                  <input
                    type="checkbox"
                    id="enable-auto-move"
                    checked={postDownloadSettings.enableAutoMove}
                    onChange={(e) => setPostDownloadSettings({ ...postDownloadSettings, enableAutoMove: e.target.checked })}
                  />
                  {t('enableAutoMove')}
                </label>
              </div>
              <div className="form-group checkbox-group">
                <label htmlFor="enable-auto-extract">
                  <input
                    type="checkbox"
                    id="enable-auto-extract"
                    checked={postDownloadSettings.enableAutoExtract}
                    onChange={(e) => setPostDownloadSettings({ ...postDownloadSettings, enableAutoExtract: e.target.checked })}
                  />
                  {t('enableAutoExtract')}
                </label>
              </div>
              <div className="form-group checkbox-group">
                <label htmlFor="enable-deep-clean">
                  <input
                    type="checkbox"
                    id="enable-deep-clean"
                    checked={postDownloadSettings.enableDeepClean}
                    onChange={(e) => setPostDownloadSettings({ ...postDownloadSettings, enableDeepClean: e.target.checked })}
                  />
                  {t('enableDeepClean')}
                </label>
              </div>
              <div className="form-group">
                <label htmlFor="monitor-interval">{t('monitorInterval')}</label>
                <input
                  type="number"
                  id="monitor-interval"
                  value={postDownloadSettings.monitorIntervalSeconds}
                  onChange={(e) => setPostDownloadSettings({ ...postDownloadSettings, monitorIntervalSeconds: parseInt(e.target.value) || 60 })}
                />
              </div>
              <div className="form-group">
                <label htmlFor="unwanted-extensions">{t('unwantedExtensions')}</label>
                <input
                  type="text"
                  id="unwanted-extensions"
                  value={postDownloadSettings.unwantedExtensions?.join(', ') || ''}
                  onChange={(e) => setPostDownloadSettings({ ...postDownloadSettings, unwantedExtensions: e.target.value.split(',').map(s => s.trim()) })}
                  placeholder=".txt, .nfo, .url"
                />
              </div>
              <div className="button-group">
                <button type="submit" className="btn-primary">{t('savePostDownload')}</button>
              </div>
            </form>
          </div>
        </>
      )}



      {currentTab === 'connections' && (
        <>
          <div className="settings-section" id="connections">
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
        </>
      )}

      {currentTab === 'language' && (
        <div className="settings-section" id="language">
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
      )}

      {currentTab === 'indexers' && (
        <>
          <div className="settings-section" id="indexers">
            <div className="section-header-with-logo">
              <h3>INDEXERS</h3>
            </div>
            <p className="settings-description">
              Manage your indexers (Prowlarr, Jackett, and External JSON Sources).
            </p>

            <div className="clients-list">
              {/* Prowlarr Card */}
              <div className={`client-card ${!prowlarrEnabled ? 'disabled' : ''}`}>
                <div className="client-info">
                  <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    <img src={prowlarrLogo} alt="Prowlarr" style={{ height: '24px' }} />
                    <h4>Prowlarr</h4>
                  </div>
                  <p>{prowlarrUrl}</p>
                </div>
                <div className="client-actions">
                  <div className="checkbox-group" style={{ marginBottom: 0 }}>
                    <label>
                      <input
                        type="checkbox"
                        checked={prowlarrEnabled}
                        onChange={toggleProwlarr}
                      />
                    </label>
                  </div>
                  <button className="btn-edit" onClick={() => setShowProwlarrModal(true)}>{t('edit')}</button>
                </div>
              </div>

              {/* Jackett Card */}
              <div className={`client-card ${!jackettEnabled ? 'disabled' : ''}`}>
                <div className="client-info">
                  <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    <img src={jackettLogo} alt="Jackett" style={{ height: '24px' }} />
                    <h4>Jackett</h4>
                  </div>
                  <p>{jackettUrl}</p>
                </div>
                <div className="client-actions">
                  <div className="checkbox-group" style={{ marginBottom: 0 }}>
                    <label>
                      <input
                        type="checkbox"
                        checked={jackettEnabled}
                        onChange={toggleJackett}
                      />
                    </label>
                  </div>
                  <button className="btn-edit" onClick={() => setShowJackettModal(true)}>{t('edit')}</button>
                </div>
              </div>

              {/* Hydra Sources Cards */}
              {hydraSources.map(source => (
                <div key={source.id} className={`client-card ${!source.enabled ? 'disabled' : ''}`}>
                  <div className="client-info">
                    <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                      <span className="category-badge" style={{ backgroundColor: 'rgba(250, 179, 135, 0.15)', color: '#fab387', border: '1px solid #fab387', fontWeight: 'bold' }}>JSON</span>
                      <h4>{source.name}</h4>
                    </div>
                    <p style={{ maxWidth: '300px', overflow: 'hidden', textOverflow: 'ellipsis' }}>{source.url}</p>
                  </div>
                  <div className="client-actions">
                    <button className="btn-square-action" onClick={() => handleDeleteHydra(source)} title={t('delete')} style={{ marginRight: 'auto' }}>
                      <FontAwesomeIcon icon={faTimes} />
                    </button>
                    <div className="checkbox-group" style={{ marginBottom: 0 }}>
                      <label>
                        <input
                          type="checkbox"
                          checked={source.enabled}
                          onChange={() => toggleHydra(source)}
                        />
                      </label>
                    </div>
                    <button className="btn-edit" onClick={() => handleOpenEditHydra(source)}>{t('edit')}</button>
                  </div>
                </div>
              ))}
            </div>

            <button className="btn-secondary" onClick={handleOpenAddHydra} style={{ marginTop: '15px' }}>
              <FontAwesomeIcon icon={faPlus} /> Add JSON Source
            </button>
          </div>

          <div className="settings-section" id="download-clients">
            <div className="section-header-with-logo">
              <img src={torrentNzbIcon} alt="Download Clients" style={{ height: '60px' }} />
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
                      <div className="checkbox-group" style={{ marginBottom: 0 }}>
                        <label>
                          <input
                            type="checkbox"
                            checked={client.enable}
                            onChange={() => toggleDownloadClient(client)}
                          />
                        </label>
                      </div>
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
        </>
      )}

      {
        showClientModal && (
          <div className="modal-overlay" onClick={() => setShowClientModal(false)}>
            <div className="modal" onClick={(e) => e.stopPropagation()}>
              <div className="modal-header">
                <h3>{editingClient ? t('editDownloadClient') : t('addDownloadClient')}</h3>
                <button className="modal-close" onClick={() => setShowClientModal(false)}>×</button>
              </div>

              <form onSubmit={handleSaveDownloadClient}>
                <div className="form-group">
                  <label>
                    <input
                      type="checkbox"
                      checked={clientForm.enable}
                      onChange={(e) => setClientForm({ ...clientForm, enable: e.target.checked })}
                    />
                    {t('enable')}
                  </label>
                </div>

                <div className="form-group">
                  <label>{t('name')}</label>
                  <input
                    type="text"
                    className="form-control"
                    value={clientForm.name}
                    onChange={(e) => setClientForm({ ...clientForm, name: e.target.value })}
                    placeholder="e.g. Deluge"
                  />
                </div>

                <div className="form-group">
                  <label>{t('implementation')}</label>
                  <select
                    className="form-control"
                    value={clientForm.implementation}
                    onChange={(e) => setClientForm({ ...clientForm, implementation: e.target.value })}
                  >
                    <option value="qBittorrent">qBittorrent</option>
                    <option value="Transmission">Transmission</option>
                    <option value="Deluge">Deluge (WebUI)</option>
                    <option value="SABnzbd">SABnzbd</option>
                    <option value="NZBGet">NZBGet</option>
                  </select>
                </div>

                <div className="form-group">
                  <label>{t('host')}</label>
                  <input
                    type="text"
                    className="form-control"
                    value={clientForm.host}
                    onChange={(e) => setClientForm({ ...clientForm, host: e.target.value })}
                    placeholder="localhost"
                  />
                </div>

                <div className="form-group">
                  <label>{t('port')}</label>
                  <input
                    type="number"
                    className="form-control"
                    value={clientForm.port}
                    onChange={(e) => setClientForm({ ...clientForm, port: parseInt(e.target.value) })}
                  />
                </div>

                {clientForm.implementation === 'Deluge' && (
                  <div className="form-group">
                    <label>
                      <input
                        type="checkbox"
                        checked={clientForm.useSsl || false}
                        onChange={(e) => setClientForm({ ...clientForm, useSsl: e.target.checked })}
                      />
                      Use SSL
                    </label>
                  </div>
                )}

                {(clientForm.implementation === 'qBittorrent' || clientForm.implementation === 'Deluge' || clientForm.implementation === 'Transmission' || clientForm.implementation === 'NZBGet') && (
                  <>
                    <div className="form-group">
                      <label>{t('username')}</label>
                      <input
                        type="text"
                        className="form-control"
                        value={clientForm.username || ''}
                        onChange={(e) => setClientForm({ ...clientForm, username: e.target.value })}
                        placeholder={clientForm.implementation === 'Deluge' ? 'Optional (WebUI usually only needs pass)' : ''}
                      />
                    </div>
                    <div className="form-group">
                      <label>{t('password')}</label>
                      <input
                        type="password"
                        className="form-control"
                        value={clientForm.password || ''}
                        onChange={(e) => setClientForm({ ...clientForm, password: e.target.value })}
                      />
                    </div>
                  </>
                )}

                {(clientForm.implementation === 'SABnzbd') && (
                  <div className="form-group">
                    <label>{t('apiKey')}</label>
                    <input
                      type="text"
                      className="form-control"
                      value={clientForm.apiKey || ''}
                      onChange={(e) => setClientForm({ ...clientForm, apiKey: e.target.value })}
                    />
                  </div>
                )}

                <div className="form-group">
                  <label>{t('category')}</label>
                  <input
                    type="text"
                    className="form-control"
                    value={clientForm.category || ''}
                    onChange={(e) => setClientForm({ ...clientForm, category: e.target.value })}
                    placeholder="playerr"
                  />
                  <small className="form-text text-muted">Optional, but recommended.</small>
                </div>

                {clientForm.implementation !== 'Deluge' && (
                  <div className="form-group">
                    <label>URL Base</label>
                    <input
                      type="text"
                      className="form-control"
                      value={clientForm.urlBase || ''}
                      onChange={(e) => setClientForm({ ...clientForm, urlBase: e.target.value })}
                      placeholder="e.g. /qbittorrent"
                    />
                  </div>
                )}

                <div className="form-group">
                  <label>Priority</label>
                  <select
                    className="form-control"
                    value={clientForm.priority}
                    onChange={(e) => setClientForm({ ...clientForm, priority: parseInt(e.target.value) })}
                  >
                    <option value={1}>High (Primary)</option>
                    <option value={50}>Last (Fallback)</option>
                  </select>
                </div>

                <div className="form-group">
                  <label>{t('remotePath')}</label>
                  <input
                    type="text"
                    value={clientForm.remotePathMapping || ''}
                    onChange={(e) => setClientForm({ ...clientForm, remotePathMapping: e.target.value })}
                    placeholder="/downloads/"
                  />
                </div>

                <div className="form-group">
                  <label>{t('localPath')}</label>
                  <div style={{ display: 'flex', gap: '10px' }}>
                    <input
                      type="text"
                      value={clientForm.localPathMapping || ''}
                      onChange={(e) => setClientForm({ ...clientForm, localPathMapping: e.target.value })}
                      placeholder="/Volumes/downloads/"
                      style={{ flex: 1 }}
                    />
                    <button
                      type="button"
                      className="btn-secondary"
                      onClick={() => {
                        // @ts-ignore
                        if (window.external && window.external.sendMessage) {
                          // We need a new handler for this specific field in Program.cs if we want native picker to work nicely
                          // For now we reuse SELECT_FOLDER:DOWNLOAD technically it sets downloadPath but here we want localPathMapping
                          // Actually, let's keep it simple for web mode first. Native mode would need Program.cs update.
                          // Wait, if I trigger polling, I need to know where to put the result.
                          // Let's implement web-mode explorer support first.
                          setActiveFolderField('clientLocalPath');
                          startFolderPolling();
                          // @ts-ignore
                          window.external.sendMessage('SELECT_FOLDER:CLIENT_LOCAL');
                        } else {
                          setActiveFolderField('clientLocalPath');
                          setShowFolderExplorer(true);
                        }
                      }}
                    >
                      📂
                    </button>
                  </div>
                </div>



                {clientTestResult && (
                  <div className={`test-result ${clientTestResult?.success === true ? 'success' : 'error'}`}>
                    {clientTestResult?.message}
                    {clientTestResult?.version && <div>{t('versionHeader')}: {clientTestResult.version}</div>}
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
                  <button
                    type="button"
                    className="btn-primary"
                    onClick={(e) => handleSaveDownloadClient(e as any)}
                  >
                    {editingClient ? t('updateClient') : t('addClientButton')}
                  </button>
                </div>
                <div className="button-group">

                </div>
              </form>
            </div>
          </div>
        )
      }


      {/* Modals */}
      <HydraSourceModal
        isOpen={showHydraModal}
        onClose={() => setShowHydraModal(false)}
        onSave={loadHydraSources}
        source={editingHydraSource}
      />

      {
        showProwlarrModal && (
          <div className="modal-overlay" onClick={() => setShowProwlarrModal(false)}>
            <div className="modal" onClick={(e) => e.stopPropagation()}>
              <div className="modal-header">
                <h3>Configure Prowlarr</h3>
                <button className="modal-close" onClick={() => setShowProwlarrModal(false)}>×</button>
              </div>
              <form onSubmit={handleSaveProwlarr}>
                <div className="form-group">
                  <label>
                    <input
                      type="checkbox"
                      checked={prowlarrEnabled}
                      onChange={(e) => setProwlarrEnabled(e.target.checked)}
                    />
                    Enable Prowlarr
                  </label>
                </div>
                <div className="form-group">
                  <label htmlFor="prowlarr-url">{t('prowlarrUrl')}</label>
                  <input
                    type="text"
                    id="prowlarr-url"
                    value={prowlarrUrl}
                    onChange={(e) => setProwlarrUrl(e.target.value)}
                    placeholder={t('prowlarrUrlPlaceholder')}
                    required
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="prowlarr-api">{t('prowlarrApiKey')}</label>
                  <input
                    type="password"
                    id="prowlarr-api"
                    value={prowlarrApiKey}
                    onChange={(e) => setProwlarrApiKey(e.target.value)}
                    placeholder={t('prowlarrApiKeyPlaceholder')}
                    required
                  />
                </div>

                {prowlarrTestResult && (
                  <div className={`test-result ${prowlarrTestResult?.success ? 'success' : 'error'}`}>
                    {prowlarrTestResult?.message}
                  </div>
                )}

                <div className="modal-actions">
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={handleTestProwlarr}
                    disabled={prowlarrTesting || !prowlarrUrl || !prowlarrApiKey}
                  >
                    {prowlarrTesting ? t('testing') : t('testConnection')}
                  </button>
                  <button type="submit" className="btn-primary">{t('save')}</button>
                </div>
              </form>
            </div>
          </div>
        )
      }

      {
        showJackettModal && (
          <div className="modal-overlay" onClick={() => setShowJackettModal(false)}>
            <div className="modal" onClick={(e) => e.stopPropagation()}>
              <div className="modal-header">
                <h3>Configure Jackett</h3>
                <button className="modal-close" onClick={() => setShowJackettModal(false)}>×</button>
              </div>
              <form onSubmit={handleSaveJackett}>
                <div className="form-group">
                  <label>
                    <input
                      type="checkbox"
                      checked={jackettEnabled}
                      onChange={(e) => setJackettEnabled(e.target.checked)}
                    />
                    Enable Jackett
                  </label>
                </div>
                <div className="form-group">
                  <label htmlFor="jackett-url">{t('jackettUrl')}</label>
                  <input
                    type="text"
                    id="jackett-url"
                    value={jackettUrl}
                    onChange={(e) => setJackettUrl(e.target.value)}
                    placeholder={t('jackettUrlPlaceholder')}
                    required
                  />
                </div>
                <div className="form-group">
                  <label htmlFor="jackett-api">{t('jackettApiKey')}</label>
                  <input
                    type="password"
                    id="jackett-api"
                    value={jackettApiKey}
                    onChange={(e) => setJackettApiKey(e.target.value)}
                    placeholder={t('jackettApiKeyPlaceholder')}
                    required
                  />
                </div>

                {jackettTestResult && (
                  <div className={`test-result ${jackettTestResult?.success ? 'success' : 'error'}`}>
                    {jackettTestResult?.message}
                  </div>
                )}

                <div className="modal-actions">
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={handleTestJackett}
                    disabled={jackettTesting || !jackettUrl || !jackettApiKey}
                  >
                    {jackettTesting ? t('testing') : t('testConnection')}
                  </button>
                  <button type="submit" className="btn-primary">{t('save')}</button>
                </div>
              </form>
            </div>
          </div>
        )
      }

      {
        showFolderExplorer && (
          <FolderExplorerModal
            initialPath={
              activeFolderField === 'media' ? folderPath :
                activeFolderField === 'download' ? downloadPath :
                  activeFolderField === 'destination' ? destinationPath :
                    (clientForm.localPathMapping || '')
            }
            onSelect={(path) => {
              if (activeFolderField === 'media') {
                setFolderPath(path);
                saveMediaConfig({ folderPath: path });
              }
              else if (activeFolderField === 'download') {
                setDownloadPath(path);
                saveMediaConfig({ downloadPath: path });
              }
              else if (activeFolderField === 'destination') {
                setDestinationPath(path);
                saveMediaConfig({ destinationPath: path });
              }
              else if (activeFolderField === 'clientLocalPath') setClientForm({ ...clientForm, localPathMapping: path });

              setShowFolderExplorer(false);
            }}
            onClose={() => setShowFolderExplorer(false)}
            language={language}
          />
        )
      }

      {
        deleteConfirmation && (
          <div className="modal-overlay" onClick={() => setDeleteConfirmation(null)}>
            <div className="modal" onClick={(e) => e.stopPropagation()}>
              <div className="modal-header">
                <h3>{t('deleteSource') || 'Delete Source'}</h3>
                <button className="modal-close" onClick={() => setDeleteConfirmation(null)}>×</button>
              </div>
              <div className="modal-content">
                <p>
                  {t('confirmDeleteSource') || 'Are you sure you want to delete this source?'}: <br />
                  <strong>{deleteConfirmation.name}</strong>
                </p>
              </div>
              <div className="modal-actions">
                <button className="btn-secondary" onClick={() => setDeleteConfirmation(null)}>{t('cancel')}</button>
                <button className="btn-delete" onClick={confirmDeleteHydra}>{t('delete')}</button>
              </div>
            </div>
          </div>
        )
      }
    </div >
  );
};

export default Settings;
