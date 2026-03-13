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

const getPlatformIcon = (game: Game) => {
  const pId = game.platformId || game.platform?.id;
  const pName = game.platform?.name?.toLowerCase() || '';
  const pSlug = game.platform?.slug?.toLowerCase() || '';
  const path = game.path?.toLowerCase() || '';

  if (game.steamId) return steamLogo;

  if (pName.includes('windows') || pName.includes('pc') || pName.includes('mac') ||
      pSlug.includes('win') || pSlug.includes('mac')) return pcIcon;

  if (pId === 130 || pName.includes('switch') || pSlug.includes('switch') ||
      path.endsWith('.nsp') || path.endsWith('.xci')) return nsIcon;

  const psIds = [7, 8, 9, 48, 167];
  if (psIds.includes(pId as number) || pName.includes('playstation') ||
      pName.match(/\bps[1-5]\b/i) || pSlug.includes('playstation') ||
      pSlug.match(/\bps[1-5]\b/i) || path.endsWith('.pkg') ||
      path.includes('.iso') || path.includes('.bin')) return psIcon;

  return null;
};

/** Returns a badge only for states worth surfacing. */
const StatusBadge: React.FC<{ status: string }> = ({ status }) => {
  switch (status) {
    case 'Downloaded':
      return <span className="gc-badge gc-badge--owned">Owned</span>;
    case 'Downloading':
      return <span className="gc-badge gc-badge--downloading">Downloading</span>;
    case 'Missing':
      return <span className="gc-badge gc-badge--missing">Missing</span>;
    default:
      return null;
  }
};

const GameCard: React.FC<GameCardProps> = ({ game, onClick, onContextMenu, onDelete }) => {
  const platformIcon = getPlatformIcon(game);
  const hasRating = typeof game.rating === 'number' && game.rating > 0;

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

        {/* Platform icon — bottom-right corner */}
        {platformIcon && (
          <div className="game-card-platform-badge" title={game.platform?.name || 'Platform'}>
            <img src={platformIcon} alt="Platform" />
          </div>
        )}

        {/* Delete button — top-left, visible on hover */}
        {onDelete && (
          <button
            className="game-card-delete-btn"
            onClick={(e) => { e.stopPropagation(); onDelete(); }}
            title="Remove from library"
          >
            ×
          </button>
        )}
      </div>

      <div className="game-card-info">
        <h3 className="game-card-title">{game.title}</h3>
        <div className="game-card-footer">
          <span className="game-card-year">{game.year || '—'}</span>
          {hasRating && <span className="gc-sep">·</span>}
          {hasRating && <span className="game-card-rating">{Math.round(game.rating!)}%</span>}
          <StatusBadge status={game.status} />
        </div>
      </div>
    </div>
  );
};

export default GameCard;
