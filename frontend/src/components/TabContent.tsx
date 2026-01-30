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
    const isActive = paths.some(path => {
        if (path === '/') return location.pathname === '/' || location.pathname === '/library';
        return location.pathname === path;
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
