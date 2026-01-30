import React from 'react';
import { useLocation } from 'react-router-dom';

interface TabContentProps {
    paths: string[];
    children: React.ReactNode;
    className?: string;
}

/**
 * TabContent keeps its children mounted but hides them with CSS if the current route 
 * does not match any of the provided paths. This implements a "Keep-Alive" pattern.
 */
const TabContent: React.FC<TabContentProps> = ({ paths, children, className = "" }) => {
    const location = useLocation();

    // Check if current location matches any of the persistent paths or their sub-paths
    const isActive = paths.some(path => {
        if (path === '/' || path === '/library') {
            // ONLY match exactly / or /library for the Library List view
            return location.pathname === '/' || location.pathname === '/library';
        }
        // For others, match prefix (e.g. /game/ matches /game/123)
        return location.pathname.startsWith(path);
    });

    return (
        <div
            className={`tab-container ${className}`}
            style={{ display: isActive ? 'block' : 'none' }}
        >
            {children}
        </div>
    );
};

export default TabContent;
