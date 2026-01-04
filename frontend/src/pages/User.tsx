import React, { useState, useEffect } from 'react';
import axios from 'axios';
import { t } from '../i18n/translations';

interface SteamProfile {
    steamId: string;
    personaName: string;
    avatarUrl: string;
    personaState: number; // 0: Offline, 1: Online, 2: Busy, 3: Away, 4: Snooze, 5: Looking to Trade, 6: Looking to Play
    gameExtraInfo?: string;
}

interface SteamLibraryStats {
    totalGames: number;
    totalMinutesPlayed: number;
    totalHoursPlayed: number;
}

interface SteamRecentGame {
    appId: number;
    name: string;
    playtime2Weeks: number;
    playtimeForever: number;
    iconUrl: string | null;
    achieved: number;
    totalAchievements: number;
    completionPercent: number;
    latestNews?: {
        title: string;
        url: string;
        feedLabel: string;
        date: string;
    };
}

const User: React.FC = () => {
    const [profile, setProfile] = useState<SteamProfile | null>(null);
    const [stats, setStats] = useState<SteamLibraryStats | null>(null);
    const [recentGames, setRecentGames] = useState<SteamRecentGame[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        const fetchData = async () => {
            try {
                const [profileRes, statsRes, recentRes] = await Promise.all([
                    axios.get('/api/v3/steam/profile'),
                    axios.get('/api/v3/steam/stats'),
                    axios.get('/api/v3/steam/recent')
                ]);
                setProfile(profileRes.data);
                setStats(statsRes.data);
                setRecentGames(recentRes.data);
            } catch (error) {
                console.error('Error fetching Steam data:', error);
            } finally {
                setLoading(false);
            }
        };

        fetchData();
    }, []);

    const getStatusColor = (state: number, game?: string) => {
        if (game) return '#a6e3a1'; // In-Game (Green)
        if (state === 1) return '#89b4fa'; // Online (Blue)
        return '#6c7086'; // Offline (Grey)
    };

    const getStatusText = (state: number, game?: string) => {
        if (game) return `${t('playing') || 'Playing'} ${game}`;
        switch (state) {
            case 0: return 'Offline';
            case 1: return 'Online';
            case 2: return 'Busy';
            case 3: return 'Away';
            default: return 'Online';
        }
    };

    return (
        <div className="library">
            <div className="library-header">
            </div>

            <div className="user-profile-header" style={{
                display: 'flex',
                alignItems: 'center',
                gap: '2rem',
                background: 'rgba(23, 26, 33, 0.8)', // Steam-like background
                padding: '2rem',
                borderRadius: '12px',
                marginBottom: '2rem',
                boxShadow: '0 4px 6px rgba(0,0,0,0.1)'
            }}>
                {loading ? (
                    <div>Loading profile...</div>
                ) : profile ? (
                    <>
                        <div className="profile-avatar" style={{ position: 'relative' }}>
                            <img
                                src={profile.avatarUrl}
                                alt={profile.personaName}
                                style={{
                                    width: '120px',
                                    height: '120px',
                                    borderRadius: '4px', // Steam avatars are usually square-ish
                                    boxShadow: `0 0 15px ${getStatusColor(profile.personaState, profile.gameExtraInfo)}`
                                }}
                            />
                        </div>
                        <div className="profile-info">
                            <h1 style={{ margin: 0, fontSize: '2.5rem', fontWeight: 700 }}>{profile.personaName}</h1>
                            <div style={{
                                fontSize: '1.2rem',
                                color: getStatusColor(profile.personaState, profile.gameExtraInfo),
                                marginTop: '0.5rem',
                                fontWeight: 500
                            }}>
                                {getStatusText(profile.personaState, profile.gameExtraInfo)}
                            </div>
                            <div style={{ marginTop: '0.5rem', color: '#6c7086' }}>
                                Steam ID: {profile.steamId}
                            </div>
                        </div>

                        {stats && (
                            <div className="profile-stats" style={{
                                marginLeft: 'auto',
                                display: 'flex',
                                gap: '3rem',
                                paddingRight: '1rem'
                            }}>
                                <div style={{ textAlign: 'center' }}>
                                    <h3 style={{ margin: '0 0 0.5rem 0', color: '#a6adc8', fontSize: '0.9rem', textTransform: 'uppercase', letterSpacing: '1px' }}>Total Games</h3>
                                    <div style={{ fontSize: '2rem', fontWeight: 700, color: '#cba6f7' }}>{stats.totalGames}</div>
                                </div>
                                <div style={{ textAlign: 'center' }}>
                                    <h3 style={{ margin: '0 0 0.5rem 0', color: '#a6adc8', fontSize: '0.9rem', textTransform: 'uppercase', letterSpacing: '1px' }}>Hours Played</h3>
                                    <div style={{ fontSize: '2rem', fontWeight: 700, color: '#f9e2af' }}>{stats.totalHoursPlayed.toLocaleString()}</div>
                                </div>
                            </div>
                        )}
                    </>
                ) : (
                    <div className="empty-library">
                        <p>Steam not connected or configured.</p>
                    </div>
                )}
            </div>



            {recentGames.length > 0 && (
                <div className="recent-activity" style={{ marginBottom: '3rem' }}>
                    <h3 style={{ color: '#fff', marginBottom: '1.5rem' }}>Recent Activity</h3>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
                        {recentGames.map(game => (
                            <div key={game.appId} style={{
                                display: 'flex',
                                alignItems: 'center',
                                background: '#1e2029',
                                padding: '1rem',
                                borderRadius: '8px',
                                gap: '1rem'
                            }}>
                                {game.iconUrl ? (
                                    <img src={game.iconUrl} alt={game.name} style={{ width: '120px', height: 'auto', borderRadius: '4px', objectFit: 'cover' }} />
                                ) : (
                                    <div style={{ width: '120px', height: '60px', background: '#333', borderRadius: '4px' }} />
                                )}
                                <div style={{ flex: 1 }}>
                                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.5rem' }}>
                                        <h4 style={{ margin: 0, fontSize: '1.1rem' }}>{game.name}</h4>
                                        <span style={{ color: '#aaa', fontSize: '0.9rem' }}>{Math.round(game.playtime2Weeks / 60)}h past 2 weeks</span>
                                    </div>

                                    {game.totalAchievements > 0 && (
                                        <div className="achievement-progress" style={{ marginBottom: '0.8rem' }}>
                                            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.8rem', color: '#aaa', marginBottom: '4px' }}>
                                                <span>Achievements</span>
                                                <span>{game.achieved} / {game.totalAchievements} ({game.completionPercent}%)</span>
                                            </div>
                                            <div style={{ width: '100%', height: '8px', background: '#333', borderRadius: '4px', overflow: 'hidden' }}>
                                                <div style={{
                                                    width: `${game.completionPercent}%`,
                                                    height: '100%',
                                                    background: 'linear-gradient(90deg, #89b4fa, #a6e3a1)',
                                                    borderRadius: '4px'
                                                }} />
                                            </div>
                                        </div>
                                    )}

                                    {game.latestNews && (
                                        <div className="game-news" style={{
                                            background: 'rgba(0,0,0,0.2)',
                                            padding: '0.8rem',
                                            borderRadius: '6px',
                                            borderLeft: '3px solid #f9e2af'
                                        }}>
                                            <div style={{ fontSize: '0.75rem', color: '#f9e2af', marginBottom: '0.3rem', textTransform: 'uppercase', letterSpacing: '0.5px' }}>
                                                {t('latestUpdate') || 'LATEST UPDATE'} • {new Date(game.latestNews.date).toLocaleDateString()}
                                            </div>
                                            <a href={game.latestNews.url} target="_blank" rel="noopener noreferrer" style={{
                                                color: '#cdd6f4',
                                                textDecoration: 'none',
                                                fontSize: '0.9rem',
                                                fontWeight: 500,
                                                display: 'block'
                                            }} className="news-link">
                                                {game.latestNews.title}
                                            </a>
                                        </div>
                                    )}
                                </div>
                            </div>
                        ))}
                    </div>
                </div>
            )}

            {!profile && (
                <div className="empty-library">
                    <h3>{t('userPageDesc')}</h3>
                </div>
            )}
        </div>
    );
};

export default User;
