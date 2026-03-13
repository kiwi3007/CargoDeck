import React from 'react';
import './About.css';
import cargodeckLogo from '../assets/cargodeck-logo-white.svg';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faGithub } from '@fortawesome/free-brands-svg-icons';
import { faHeart, faBoxArchive, faNetworkWired, faSave, faSearch, faMicrochip, faRotate } from '@fortawesome/free-solid-svg-icons';

const features = [
  {
    icon: faBoxArchive,
    title: 'Game Library',
    desc: 'Self-hosted game library manager inspired by Radarr & Sonarr. Automatically organises, extracts, and imports game files from your download clients.',
  },
  {
    icon: faSearch,
    title: 'Indexer Search',
    desc: 'Search Prowlarr, Jackett, and Hydra indexers for game releases. Filter by quality and format, then send directly to qBittorrent, Transmission, SABnzbd, and more.',
  },
  {
    icon: faNetworkWired,
    title: 'Remote Devices',
    desc: 'Install and manage games on remote devices — Steam Deck, HTPC, or any Linux/Windows machine — over the network via lightweight agents.',
  },
  {
    icon: faMicrochip,
    title: 'Wine & Proton',
    desc: 'Full Wine and Proton support. Auto-detects the best available runner per device, generates launch scripts, and applies crack files automatically.',
  },
  {
    icon: faSave,
    title: 'Save Sync',
    desc: 'Syncs game saves across devices in real-time using fsnotify. Detects conflicts, suppresses false positives after restores, and retains the last 10 snapshots per device.',
  },
  {
    icon: faRotate,
    title: 'Update Tracking',
    desc: 'Monitors your indexers for newer versions of installed games. Parses versions from release names, PE binaries, GOG info files, and engine-specific metadata.',
  },
];

const About: React.FC = () => {
  return (
    <div className="about-page">
      <div className="about-hero">
        <div className="about-eyebrow">CARGODECK · v0.1</div>
        <div className="about-logo-row">
          <img src={cargodeckLogo} alt="" className="about-eye" />
          <h1 className="about-title">CargoDeck</h1>
        </div>
        <p className="about-subtitle">
          Self-hosted game library manager &amp; remote installer.<br />
          Like Radarr — but for games, with Steam Deck in mind.
        </p>
      </div>

      <div className="about-features">
        {features.map((f) => (
          <div className="about-feature-card" key={f.title}>
            <div className="about-feature-icon">
              <FontAwesomeIcon icon={f.icon} />
            </div>
            <div className="about-feature-body">
              <h3>{f.title}</h3>
              <p>{f.desc}</p>
            </div>
          </div>
        ))}
      </div>

      <div className="about-footer">
        <div className="about-footer-links">
          <a
            href="https://github.com/kiwi3007/CargoDeck"
            target="_blank"
            rel="noopener noreferrer"
            className="about-link about-link--github"
          >
            <FontAwesomeIcon icon={faGithub} />
            GitHub
          </a>
          <a
            href="https://github.com/Maikboarder/Playerr"
            target="_blank"
            rel="noopener noreferrer"
            className="about-link about-link--fork"
          >
            <FontAwesomeIcon icon={faGithub} />
            Forked from Maikboarder/Playerr
          </a>
        </div>
        <p className="about-license">Released under the GNU General Public License v3.0</p>
        <p className="about-ai-disclaimer">This project is developed with the assistance of AI.</p>
      </div>
    </div>
  );
};

export default About;
