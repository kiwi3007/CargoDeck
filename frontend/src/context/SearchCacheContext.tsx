import React, { createContext, useContext, useState, ReactNode } from 'react';

// Reusing the TorrentResult interface or a simplified version standard
// Ideally we should import it from a shared types file, but for now defining strict any to avoid circular deps with GameDetails if not extracted
interface SearchCacheData {
    results: any[]; // using any temporarily to avoid duplicating the huge TorrentResult interface or need to extract it
    timestamp: number;
}

interface SearchCacheContextType {
    cache: Record<number, SearchCacheData>;
    setCacheForGame: (gameId: number, results: any[]) => void;
    getCacheForGame: (gameId: number) => any[] | null;
    clearCache: () => void;
}

const SearchCacheContext = createContext<SearchCacheContextType | undefined>(undefined);

export const SearchCacheProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
    const [cache, setCache] = useState<Record<number, SearchCacheData>>({});

    const setCacheForGame = (gameId: number, results: any[]) => {
        setCache(prev => ({
            ...prev,
            [gameId]: {
                results,
                timestamp: Date.now()
            }
        }));
    };

    const getCacheForGame = (gameId: number) => {
        return cache[gameId]?.results || null;
    };

    const clearCache = () => {
        setCache({});
    };

    return (
        <SearchCacheContext.Provider value={{ cache, setCacheForGame, getCacheForGame, clearCache }}>
            {children}
        </SearchCacheContext.Provider>
    );
};

export const useSearchCache = () => {
    const context = useContext(SearchCacheContext);
    if (context === undefined) {
        throw new Error('useSearchCache must be used within a SearchCacheProvider');
    }
    return context;
};
