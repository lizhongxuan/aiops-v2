import { createContext, useCallback, useContext, useEffect, useMemo, useState, type PropsWithChildren, type ReactNode } from "react";

type AppShellChromeContextValue = {
  headerContent: ReactNode;
  headerTitle: string | null;
  headerDescription: string | null;
  headerActions: ReactNode;
  workspaceChrome: AppShellWorkspaceChrome;
};

type AppShellChromeWriterContextValue = {
  setHeaderContent: (content: ReactNode) => void;
  setHeaderPageChrome: (chrome: AppShellPageChrome) => void;
  setWorkspaceChrome: (chrome: AppShellWorkspaceChrome) => void;
};

type AppShellPageChrome = {
  title?: string | null;
  description?: string | null;
  actions?: ReactNode | null;
};

export type AppShellMode = "default" | "coroot";

export type AppShellWorkspaceSidebarState = {
  collapsed: boolean;
  toggleCollapsed: () => void;
};

export type AppShellWorkspaceChrome = {
  mode?: AppShellMode;
  sidebar?: ReactNode | ((state: AppShellWorkspaceSidebarState) => ReactNode) | null;
  hideHeader?: boolean;
  mainClassName?: string;
};

const noop = () => {};

const AppShellChromeContext = createContext<AppShellChromeContextValue>({
  headerContent: null,
  headerTitle: null,
  headerDescription: null,
  headerActions: null,
  workspaceChrome: { mode: "default" },
});

const AppShellChromeWriterContext = createContext<AppShellChromeWriterContextValue>({
  setHeaderContent: noop,
  setHeaderPageChrome: noop,
  setWorkspaceChrome: noop,
});

export function AppShellChromeProvider({ children }: PropsWithChildren) {
  const [headerContent, setHeaderContentState] = useState<ReactNode>(null);
  const [headerTitle, setHeaderTitle] = useState<string | null>(null);
  const [headerDescription, setHeaderDescription] = useState<string | null>(null);
  const [headerActions, setHeaderActions] = useState<ReactNode>(null);
  const [workspaceChrome, setWorkspaceChromeState] = useState<AppShellWorkspaceChrome>({ mode: "default" });

  const setHeaderContent = useCallback((content: ReactNode) => {
    setHeaderContentState(content);
  }, []);

  const setHeaderPageChrome = useCallback((chrome: AppShellPageChrome) => {
    if (Object.prototype.hasOwnProperty.call(chrome, "title")) setHeaderTitle(chrome.title ?? null);
    if (Object.prototype.hasOwnProperty.call(chrome, "description")) setHeaderDescription(chrome.description ?? null);
    if (Object.prototype.hasOwnProperty.call(chrome, "actions")) setHeaderActions(chrome.actions ?? null);
  }, []);

  const setWorkspaceChrome = useCallback((chrome: AppShellWorkspaceChrome) => {
    setWorkspaceChromeState({ mode: "default", ...chrome });
  }, []);

  const value = useMemo(
    () => ({
      headerContent,
      headerTitle,
      headerDescription,
      headerActions,
      workspaceChrome,
    }),
    [headerActions, headerContent, headerDescription, headerTitle, workspaceChrome],
  );
  const writerValue = useMemo(
    () => ({
      setHeaderContent,
      setHeaderPageChrome,
      setWorkspaceChrome,
    }),
    [setHeaderContent, setHeaderPageChrome, setWorkspaceChrome],
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

export function useRegisterAppShellWorkspace(chrome: AppShellWorkspaceChrome) {
  const { setWorkspaceChrome } = useContext(AppShellChromeWriterContext);

  useEffect(() => {
    setWorkspaceChrome(chrome);
    return () => setWorkspaceChrome({ mode: "default", sidebar: null, hideHeader: false, mainClassName: undefined });
  }, [chrome, setWorkspaceChrome]);
}
