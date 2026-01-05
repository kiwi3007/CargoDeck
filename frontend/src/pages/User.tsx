import React, { useState, useEffect } from 'react';
import axios from 'axios';
import { t } from '../i18n/translations';

interface SteamProfile {
    steamId: string;
    personaName: string;
    avatarUrl: string;
    personaState: number; // 0: Offline, 1: Online, 2: Busy, 3: Away, 4: Snooze, 5: Looking to Trade, 6: Looking to Play
    gameExtraInfo?: string;
    realName?: string;
    countryCode?: string;
    accountCreated?: string;
    level?: number;
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

interface SteamFriend {
    steamId: string;
    personaName: string;
    avatarUrl: string;
    personaState: number; // 0=Offline, 1=Online, etc
    gameExtraInfo: string;
}

const User: React.FC = () => {
    const [profile, setProfile] = useState<SteamProfile | null>(null);
    const [stats, setStats] = useState<SteamLibraryStats | null>(null);
    const [recentGames, setRecentGames] = useState<SteamRecentGame[]>([]);
    const [friends, setFriends] = useState<SteamFriend[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        const fetchData = async () => {
            try {
                // Fetch independently to allow partial loading
                try {
                    const profileRes = await axios.get('/api/v3/steam/profile');
                    console.log('Steam Profile Data:', profileRes.data); // DEBUG: Check level
                    if (profileRes.data) setProfile(profileRes.data);
                } catch (e) {
                    console.warn('Failed to load profile', e);
                }

                try {
                    const statsRes = await axios.get('/api/v3/steam/stats');
                    if (statsRes.data) setStats(statsRes.data);
                } catch (e) {
                    console.warn('Failed to load stats', e);
                }

                try {
                    const recentRes = await axios.get('/api/v3/steam/recent');
                    // Ensure it is an array
                    if (Array.isArray(recentRes.data)) {
                        setRecentGames(recentRes.data);
                    } else {
                        setRecentGames([]);
                    }
                } catch (e) {
                    console.warn('Failed to load recent games', e);
                    setRecentGames([]);
                }

                try {
                    const friendRes = await axios.get('/api/v3/steam/friends');
                    if (Array.isArray(friendRes.data)) {
                        setFriends(friendRes.data);
                    }
                } catch (e) {
                    console.warn('Failed to load friends', e);
                }

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
                            <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
                                <h1 style={{ margin: 0, fontSize: '2.5rem', fontWeight: 700 }}>{profile.personaName}</h1>
                                {profile.countryCode && (
                                    <span style={{ fontSize: '1.5rem' }}>
                                        {/* Simple logic to detect country roughly or just show code. For now showing code is safer. */}
                                        <img
                                            src={`https://flagcdn.com/24x18/${profile.countryCode.toLowerCase()}.png`}
                                            alt={profile.countryCode}
                                            title={profile.countryCode}
                                            style={{ borderRadius: '2px' }}
                                        />
                                    </span>
                                )}
                                {profile.level !== undefined && (
                                    <div
                                        title={`Steam Level ${profile.level}`}
                                        style={{
                                            border: '2px solid #f2f2f2',
                                            borderRadius: '50%',
                                            width: '32px',
                                            height: '32px',
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            fontWeight: 'bold',
                                            fontSize: '0.9rem',
                                            color: '#f2f2f2',
                                            marginLeft: '0.5rem'
                                        }}
                                    >
                                        {profile.level}
                                    </div>
                                )}
                            </div>

                            <div style={{
                                fontSize: '1.2rem',
                                color: getStatusColor(profile.personaState, profile.gameExtraInfo),
                                marginTop: '0.5rem',
                                fontWeight: 500
                            }}>
                                {getStatusText(profile.personaState, profile.gameExtraInfo)}
                            </div>

                            {/* NEW: Real Name & Member Since */}
                            {(profile.realName || profile.accountCreated) && (
                                <div style={{ marginTop: '0.5rem', color: '#c5c5c5', fontSize: '0.9rem', display: 'flex', gap: '1rem' }}>
                                    {profile.realName && <span>{profile.realName}</span>}
                                    {profile.accountCreated && (
                                        <span>
                                            Member since {new Date(profile.accountCreated).getFullYear()}
                                        </span>
                                    )}
                                </div>
                            )}

                            <div style={{ marginTop: '0.2rem', color: '#6c7086', fontSize: '0.8rem' }}>
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
                                {(() => { console.log(`Game: ${game.name}`, game.latestNews); return null; })()}
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
                                        <div style={{ marginTop: '0.8rem', fontSize: '0.9rem', borderTop: '1px solid #333', paddingTop: '0.5rem' }}>
                                            <span style={{ color: '#89b4fa', fontWeight: 'bold', marginRight: '0.5rem', textTransform: 'uppercase', fontSize: '0.75rem' }}>
                                                {game.latestNews.feedLabel || 'NEWS'}
                                            </span>
                                            <a
                                                href={game.latestNews.url}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                style={{ color: '#cdd6f4', textDecoration: 'none', fontWeight: 500 }}
                                            >
                                                {game.latestNews.title}
                                            </a>
                                            <span style={{ color: '#6c7086', marginLeft: '0.5rem', fontSize: '0.8rem' }}>
                                                {new Date(game.latestNews.date).toLocaleDateString()}
                                            </span>
                                        </div>
                                    )}
                                </div>
                            </div>
                        ))}
                    </div>
                </div >
            )}

            {/* Friends Section */}
            {
                friends.length > 0 && (
                    <div className="friends-list" style={{ marginBottom: '3rem' }}>
                        <h3 style={{ color: '#fff', marginBottom: '1.5rem' }}>Friends ({friends.length})</h3>
                        <div style={{
                            display: 'grid',
                            gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
                            gap: '1rem'
                        }}>
                            {friends.map(friend => (
                                <div key={friend.steamId} style={{
                                    background: '#1e2029',
                                    padding: '0.8rem',
                                    borderRadius: '8px',
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: '1rem',
                                    border: friend.gameExtraInfo ? '1px solid #a6e3a1' : (friend.personaState === 1 ? '1px solid #89b4fa' : '1px solid transparent')
                                }}>
                                    <div style={{ position: 'relative' }}>
                                        <img
                                            src={friend.avatarUrl}
                                            alt={friend.personaName}
                                            style={{ width: '48px', height: '48px', borderRadius: '4px' }}
                                        />
                                    </div>
                                    <div style={{ overflow: 'hidden' }}>
                                        <div style={{
                                            color: '#fff',
                                            fontWeight: 600,
                                            whiteSpace: 'nowrap',
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis'
                                        }}>
                                            {friend.personaName}
                                        </div>
                                        <div style={{
                                            color: getStatusColor(friend.personaState, friend.gameExtraInfo),
                                            fontSize: '0.8rem',
                                            whiteSpace: 'nowrap',
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis'
                                        }}>
                                            {friend.gameExtraInfo ? `Playing ${friend.gameExtraInfo}` : getStatusText(friend.personaState)}
                                        </div>
                                    </div>
                                </div>
                            ))}
                        </div>
                    </div>
                )
            }

            {
                !profile && (
                    <div className="empty-library">
                        <h3>{t('userPageDesc')}</h3>
                    </div>
                )
            }
        </div >
    );
};

export default User;
