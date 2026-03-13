import { Link, NavLink } from 'react-router-dom';
import { useTranslation } from '../i18n/translations';
import { useState } from 'react';
import './Navigation.css';
import cargodeckLogo from '../assets/cargodeck-logo-white.svg';

import { useUI } from '../context/UIContext';

const Navigation: React.FC = () => {
  const { t } = useTranslation();
  const { lastLibraryPath, lastSettingsPath } = useUI();
  const [showSettingsDropdown, setShowSettingsDropdown] = useState(false);

  const handleDropdownItemClick = (tabName: string) => {
    setShowSettingsDropdown(false);
    // Hash change will be handled by Link to="/settings#..."
  };

  return (
    <nav className="navigation">
      <div className="nav-brand">
        <Link to="/library" className="nav-logo-link">
          <img src={cargodeckLogo} alt="" className="nav-logo-eye" />
          <span className="nav-brand-name">CargoDeck</span>
        </Link>
      </div>
      <ul className="nav-links">
        <li><NavLink to={lastLibraryPath}>{t('library')}</NavLink></li>
        <li><NavLink to="/devices">Devices</NavLink></li>
        <li><NavLink to="/status">Downloads</NavLink></li>
        <li
          className="nav-item-dropdown"
          onMouseEnter={() => setShowSettingsDropdown(true)}
          onMouseLeave={() => setShowSettingsDropdown(false)}
        >
          <NavLink to={lastSettingsPath} className="nav-link-with-arrow">
            {t('settings')} <span className={`dropdown-arrow ${showSettingsDropdown ? 'open' : ''}`}>▾</span>
          </NavLink>
          <div className={`dropdown-menu ${showSettingsDropdown ? 'show' : ''}`}>
            <Link to="/settings#media" onClick={() => handleDropdownItemClick('media')}>{t('settingsMedia')}</Link>
            <Link to="/settings#connections" onClick={() => handleDropdownItemClick('connections')}>{t('settingsConnections')}</Link>
            <Link to="/settings#indexers" onClick={() => handleDropdownItemClick('indexers')}>{t('settingsIndexers')}</Link>
            <Link to="/settings#clients" onClick={() => handleDropdownItemClick('clients')}>{t('settingsClients')}</Link>
            <Link to="/settings#agents" onClick={() => handleDropdownItemClick('agents')}>Agents</Link>
            <Link to="/settings#accounts" onClick={() => handleDropdownItemClick('accounts')}>Accounts</Link>
            <Link to="/settings#updates" onClick={() => handleDropdownItemClick('updates')}>Updates</Link>
          </div>
        </li>
        <li><NavLink to="/about">{t('about')}</NavLink></li>
      </ul>
      <div className="nav-branch-tag">v0.1</div>
    </nav>
  );
};

export default Navigation;
