import React, { createContext, useContext, useEffect, useMemo, useState } from 'react';
import { DEFAULT_LOCALE, type Locale, interpolate, localeOptions, translate } from '../lib/i18n';

const STORAGE_KEY = 'clawmanager_locale';

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: string, variables?: Record<string, string | number>) => string;
  localeOptions: Array<{ value: Locale; label: string }>;
}

const I18nContext = createContext<I18nContextValue | undefined>(undefined);

export const I18nProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [locale, setLocaleState] = useState<Locale>(() => {
    const stored = window.localStorage.getItem(STORAGE_KEY) as Locale | null;
    return stored ?? DEFAULT_LOCALE;
  });

  useEffect(() => {
    window.localStorage.setItem(STORAGE_KEY, locale);
    document.documentElement.lang = locale;
  }, [locale]);

  const value = useMemo<I18nContextValue>(() => ({
    locale,
    setLocale: (nextLocale) => setLocaleState(nextLocale),
    t: (key, variables) => {
      const text = translate(locale, key) ?? translate(DEFAULT_LOCALE, key) ?? key;
      return interpolate(text, variables);
    },
    localeOptions,
  }), [locale]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
};

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error('useI18n must be used within I18nProvider');
  }
  return context;
}
