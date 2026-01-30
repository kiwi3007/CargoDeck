import { Link, NavLink, useLocation } from 'react-router-dom';
import { useTranslation } from '../i18n/translations';
import { useState } from 'react';
import './Navigation.css';
import navEye from '../assets/nav_eye.png';
import navLetters from '../assets/nav_letters.png';

import { useUI } from '../context/UIContext';

const Navigation: React.FC = () => {
  const { t } = useTranslation();
  const { toggleKofi, lastLibraryPath, lastSettingsPath } = useUI();
  const [showSettingsDropdown, setShowSettingsDropdown] = useState(false);
  const location = useLocation();

  const handleDropdownItemClick = (tabName: string) => {
    setShowSettingsDropdown(false);
    // Hash change will be handled by Link to="/settings#..."
  };

  return (
    <nav className="navigation">
      <div className="nav-brand">
        <div className="nav-logo-link" onClick={toggleKofi} style={{ cursor: 'pointer' }}>
          <img src={navEye} alt="" className="nav-logo-eye" />
          <img src={navLetters} alt="Playerr" className="nav-logo-letters" />
        </div>
      </div>
      <ul className="nav-links">
        <li><NavLink to={lastLibraryPath}>{t('library')}</NavLink></li>
        <li><NavLink to="/status">{t('status')}</NavLink></li>
        <li><NavLink to="/user">{t('user')}</NavLink></li>
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
            <Link to="/settings#language" onClick={() => handleDropdownItemClick('language')}>{t('settingsLanguage')}</Link>
            <Link to="/settings#advanced" onClick={() => handleDropdownItemClick('advanced')}>{t('settingsAdvanced') || 'Advanced'}</Link>
          </div>
        </li>
        <li><NavLink to="/about">{t('about')}</NavLink></li>
      </ul>
      <div className="nav-branch-tag" style={{
        fontSize: '10px',
        opacity: 0.6,
        padding: '2px 6px',
        border: '1px solid currentColor',
        borderRadius: '4px',
        marginLeft: '10px',
        whiteSpace: 'nowrap'
      }}>
        BETA Release
      </div>
    </nav>
  );
};

export default Navigation;
