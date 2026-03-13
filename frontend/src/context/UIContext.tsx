import React, { createContext, useContext, useState, ReactNode } from 'react';

interface UIContextType {
    lastLibraryPath: string;
    setLastLibraryPath: (path: string) => void;
    lastSettingsPath: string;
    setLastSettingsPath: (path: string) => void;
}

const UIContext = createContext<UIContextType | undefined>(undefined);

export const UIProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
    const [lastLibraryPath, setLastLibraryPath] = useState('/library');
    const [lastSettingsPath, setLastSettingsPath] = useState('/settings');

    return (
        <UIContext.Provider value={{
            lastLibraryPath, setLastLibraryPath,
            lastSettingsPath, setLastSettingsPath
        }}>
            {children}
        </UIContext.Provider>
    );
};

export const useUI = () => {
    const context = useContext(UIContext);
    if (context === undefined) {
        throw new Error('useUI must be used within a UIProvider');
    }
    return context;
};
