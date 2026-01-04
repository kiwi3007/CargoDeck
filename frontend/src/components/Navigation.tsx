import { Link, NavLink } from 'react-router-dom';
import { useTranslation } from '../i18n/translations';
import './Navigation.css';
import navEye from '../assets/nav_eye.png';
import navLetters from '../assets/nav_letters.png';

const Navigation: React.FC = () => {
  const { t } = useTranslation();

  return (
    <nav className="navigation">
      <div className="nav-brand">
        <Link to="/library" className="nav-logo-link">
          <img src={navEye} alt="" className="nav-logo-eye" />
          <img src={navLetters} alt="Playerr" className="nav-logo-letters" />
        </Link>
      </div>
      <ul className="nav-links">
        <li><NavLink to="/library">{t('library')}</NavLink></li>
        <li><NavLink to="/user">{t('user')}</NavLink></li>
        <li><NavLink to="/settings">{t('settings')}</NavLink></li>
        <li><NavLink to="/about">{t('about')}</NavLink></li>
      </ul>
    </nav>
  );
};

export default Navigation;
