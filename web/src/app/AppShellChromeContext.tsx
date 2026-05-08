import { createContext, useContext, useEffect, useMemo, useState, type PropsWithChildren, type ReactNode } from "react";

type AppShellChromeContextValue = {
  headerContent: ReactNode;
  setHeaderContent: (content: ReactNode) => void;
};

const noop = () => {};

const AppShellChromeContext = createContext<AppShellChromeContextValue>({
  headerContent: null,
  setHeaderContent: noop,
});

export function AppShellChromeProvider({ children }: PropsWithChildren) {
  const [headerContent, setHeaderContent] = useState<ReactNode>(null);
  const value = useMemo(
    () => ({
      headerContent,
      setHeaderContent,
    }),
    [headerContent],
  );

  return <AppShellChromeContext.Provider value={value}>{children}</AppShellChromeContext.Provider>;
}

export function useAppShellChrome() {
  return useContext(AppShellChromeContext);
}

export function useRegisterAppShellHeader(content: ReactNode) {
  const { setHeaderContent } = useAppShellChrome();

  useEffect(() => {
    setHeaderContent(content);
    return () => setHeaderContent(null);
  }, [content, setHeaderContent]);
}
