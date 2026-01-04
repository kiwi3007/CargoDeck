import steamLogo from '../assets/steam_logo.png';
import pcIcon from '../assets/pc_icon.png';
import nsIcon from '../assets/ns_icon.png';
import psIcon from '../assets/ps_icon.png';
import './GameCard.css';

interface Game {
  id: number;
  title: string;
  year: number;
  overview?: string;
  images: {
    coverUrl?: string;
    backgroundUrl?: string;
  };
  rating?: number;
  genres: string[];
  platformId?: number;
  platform?: {
    id?: number;
    name: string;
    slug?: string;
  };
  status: string;
  steamId?: number;
  path?: string;
}

interface GameCardProps {
  game: Game;
  onClick?: () => void;
  onContextMenu?: (e: React.MouseEvent) => void;
  onDelete?: () => void;
}

const GameCard: React.FC<GameCardProps> = ({ game, onClick, onContextMenu, onDelete }) => {
  const getStatusColor = (status: string) => {
    switch (status) {
      case 'Downloaded': return '#a6e3a1';
      case 'Downloading': return '#89b4fa';
      case 'Missing': return '#f38ba8';
      default: return '#cdd6f4';
    }
  };

  const getPlatformIcon = (game: Game) => {
    const pId = game.platformId || game.platform?.id;
    const pName = game.platform?.name?.toLowerCase() || '';
    const pSlug = game.platform?.slug?.toLowerCase() || '';
    const path = game.path?.toLowerCase() || '';

    // 1. Steam (Prioritized)
    if (game.steamId) {
      return steamLogo;
    }

    // 2. PC (Windows + Mac + Generic Desktop)
    if (pName.includes('windows') || pName.includes('pc') || pName.includes('mac') || pSlug.includes('win') || pSlug.includes('mac')) {
      return pcIcon;
    }

    // 2. Switch (NS)
    if (pId === 130 || pName.includes('switch') || pSlug.includes('switch') || path.endsWith('.nsp') || path.endsWith('.xci')) {
      return nsIcon;
    }

    // 3. PlayStation (PS)
    const psIds = [7, 8, 9, 48, 167];
    if (psIds.includes(pId as number) ||
      pName.includes('playstation') ||
      pName.match(/\bps[1-5]\b/i) ||
      pSlug.includes('playstation') ||
      pSlug.match(/\bps[1-5]\b/i) ||
      path.endsWith('.pkg') ||
      path.includes('.iso') ||
      path.includes('.bin')) {
      return psIcon;
    }

    return null;
  };

  const platformIcon = getPlatformIcon(game);

  return (
    <div className="game-card" onClick={onClick} onContextMenu={onContextMenu}>
      <div className="game-card-poster">
        {game.images.coverUrl ? (
          <img src={game.images.coverUrl} alt={game.title} />
        ) : (
          <div className="game-card-placeholder">
            <span>?</span>
          </div>
        )}

        {/* Platform Badge Overlay */}
        {platformIcon && (
          <div className="game-card-platform-badge" title={game.platform?.name || 'Platform'}>
            <img src={platformIcon} alt="Platform" />
          </div>
        )}
        <div className="game-card-overlay">
          <div className="game-card-rating">
            {game.rating ? `${Math.round(game.rating)}%` : 'N/A'}
          </div>
          {onDelete && (
            <button
              className="game-card-delete-btn"
              onClick={(e) => {
                e.stopPropagation();
                onDelete();
              }}
              title="Eliminar de la biblioteca"
            >
              ×
            </button>
          )}
        </div>
      </div>
      <div className="game-card-info">
        <h3 className="game-card-title">{game.title}</h3>
        <div className="game-card-meta">
          <span className="game-card-year">{game.year}</span>
          {game.platform && (
            <span className="game-card-platform">{game.platform.name}</span>
          )}
        </div>
        <div
          className="game-card-status"
          style={{ backgroundColor: getStatusColor(game.status) }}
        >
          {game.status}
        </div>
      </div>
    </div>
  );
};

export default GameCard;
