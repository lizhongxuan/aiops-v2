import { createContext, useCallback, useContext, useEffect, useMemo, useState, type PropsWithChildren, type ReactNode } from "react";

type AppShellChromeContextValue = {
  headerContent: ReactNode;
  headerTitle: string | null;
  headerDescription: string | null;
  headerActions: ReactNode;
};

type AppShellChromeWriterContextValue = {
  setHeaderContent: (content: ReactNode) => void;
  setHeaderPageChrome: (chrome: AppShellPageChrome) => void;
};

type AppShellPageChrome = {
  title?: string | null;
  description?: string | null;
  actions?: ReactNode | null;
};

const noop = () => {};

const AppShellChromeContext = createContext<AppShellChromeContextValue>({
  headerContent: null,
  headerTitle: null,
  headerDescription: null,
  headerActions: null,
});

const AppShellChromeWriterContext = createContext<AppShellChromeWriterContextValue>({
  setHeaderContent: noop,
  setHeaderPageChrome: noop,
});

export function AppShellChromeProvider({ children }: PropsWithChildren) {
  const [headerContent, setHeaderContentState] = useState<ReactNode>(null);
  const [headerTitle, setHeaderTitle] = useState<string | null>(null);
  const [headerDescription, setHeaderDescription] = useState<string | null>(null);
  const [headerActions, setHeaderActions] = useState<ReactNode>(null);

  const setHeaderContent = useCallback((content: ReactNode) => {
    setHeaderContentState(content);
  }, []);

  const setHeaderPageChrome = useCallback((chrome: AppShellPageChrome) => {
    if (Object.prototype.hasOwnProperty.call(chrome, "title")) setHeaderTitle(chrome.title ?? null);
    if (Object.prototype.hasOwnProperty.call(chrome, "description")) setHeaderDescription(chrome.description ?? null);
    if (Object.prototype.hasOwnProperty.call(chrome, "actions")) setHeaderActions(chrome.actions ?? null);
  }, []);

  const value = useMemo(
    () => ({
      headerContent,
      headerTitle,
      headerDescription,
      headerActions,
    }),
    [headerActions, headerContent, headerDescription, headerTitle],
  );
  const writerValue = useMemo(
    () => ({
      setHeaderContent,
      setHeaderPageChrome,
    }),
    [setHeaderContent, setHeaderPageChrome],
  );

  return (
    <AppShellChromeContext.Provider value={value}>
      <AppShellChromeWriterContext.Provider value={writerValue}>{children}</AppShellChromeWriterContext.Provider>
    </AppShellChromeContext.Provider>
  );
}

export function useAppShellChrome() {
  return {
    ...useContext(AppShellChromeContext),
    ...useContext(AppShellChromeWriterContext),
  };
}

export function useRegisterAppShellHeader(content: ReactNode) {
  const { setHeaderContent } = useContext(AppShellChromeWriterContext);

  useEffect(() => {
    setHeaderContent(content);
    return () => setHeaderContent(null);
  }, [content, setHeaderContent]);
}

export function useRegisterAppShellPageChrome({ title, description, actions }: AppShellPageChrome) {
  const { setHeaderPageChrome } = useContext(AppShellChromeWriterContext);

  useEffect(() => {
    setHeaderPageChrome({ title, description, actions });
    return () => setHeaderPageChrome({ title: null, description: null, actions: null });
  }, [actions, description, setHeaderPageChrome, title]);
}

export function useRegisterAppShellHeaderActions(actions: ReactNode) {
  const { setHeaderPageChrome } = useContext(AppShellChromeWriterContext);

  useEffect(() => {
    setHeaderPageChrome({ actions });
    return () => setHeaderPageChrome({ actions: null });
  }, [actions, setHeaderPageChrome]);
}
