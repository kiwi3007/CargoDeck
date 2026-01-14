import { Link, NavLink } from 'react-router-dom';
import { useTranslation } from '../i18n/translations';
import './Navigation.css';
import navEye from '../assets/nav_eye.png';
import navLetters from '../assets/nav_letters.png';

import { useUI } from '../context/UIContext';

const Navigation: React.FC = () => {
  const { t } = useTranslation();
  const { toggleKofi } = useUI();

  return (
    <nav className="navigation">
      <div className="nav-brand">
        <div className="nav-logo-link" onClick={toggleKofi} style={{ cursor: 'pointer' }}>
          <img src={navEye} alt="" className="nav-logo-eye" />
          <img src={navLetters} alt="Playerr" className="nav-logo-letters" />
        </div>
      </div>
      <ul className="nav-links">
        <li><NavLink to="/library">{t('library')}</NavLink></li>
        <li><NavLink to="/status">{t('status')}</NavLink></li>
        <li><NavLink to="/user">{t('user')}</NavLink></li>
        <li><NavLink to="/settings">{t('settings')}</NavLink></li>
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
