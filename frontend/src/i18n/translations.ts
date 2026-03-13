import { en } from './locales/en';

export type TranslationKey = keyof typeof en;

export const t = (key: TranslationKey): string => {
    const val = en[key];
    return val !== undefined ? val : key;
};

export const useTranslation = () => {
    return { t };
};
