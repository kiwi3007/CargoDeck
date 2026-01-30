import { es } from './locales/es';
import { en } from './locales/en';
import { fr } from './locales/fr';
import { de } from './locales/de';
import { ru } from './locales/ru';
import { zh } from './locales/zh';
import { ja } from './locales/ja';

export const translations = {
    es,
    en,
    fr,
    de,
    ru,
    zh,
    ja
} as const;

export type TranslationKey = keyof typeof translations.en;
export type Language = keyof typeof translations;

export const setLanguage = (lang: Language) => {
    localStorage.setItem('playerr_language', lang);
    window.dispatchEvent(new Event('languageChange'));
};

export const getLanguage = (): Language => {
    const savedLang = localStorage.getItem('playerr_language') as Language;
    if (savedLang && translations[savedLang]) {
        return savedLang;
    }
    return 'en'; // Default
};

export const t = (key: TranslationKey, lang?: Language): string => {
    const language = lang || getLanguage();
    // @ts-ignore
    const val = translations[language][key];
    if (val !== undefined) return val;

    // @ts-ignore
    const enVal = translations['en'][key];
    if (enVal !== undefined) return enVal;

    return key;
};

import { useState, useEffect } from 'react';

export const useTranslation = () => {
    const [language, setLangState] = useState<Language>(getLanguage());

    useEffect(() => {
        const handleLanguageChange = () => {
            setLangState(getLanguage());
        };

        window.addEventListener('languageChange', handleLanguageChange);
        return () => {
            window.removeEventListener('languageChange', handleLanguageChange);
        };
    }, []);

    return {
        t: (key: TranslationKey) => t(key, language),
        language,
        setLanguage
    };
};
