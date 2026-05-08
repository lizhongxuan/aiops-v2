import { useEffect, useRef, useState } from "react";

type MonacoModule = typeof import("monaco-editor/esm/vs/editor/editor.api");
type MonacoEditor = import("monaco-editor/esm/vs/editor/editor.api").editor.IStandaloneCodeEditor;
type MonacoDisposable = import("monaco-editor/esm/vs/editor/editor.api").IDisposable;

interface CodeEditorProps {
  value: string;
  language?: string;
  readonly?: boolean;
  height?: string;
  placeholder?: string;
  onChange?: (value: string) => void;
}

export default function CodeEditor({
  value,
  language = "json",
  readonly = false,
  height = "240px",
  placeholder = "",
  onChange,
}: CodeEditorProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const editorRef = useRef<MonacoEditor | null>(null);
  const monacoRef = useRef<MonacoModule | null>(null);
  const listenerRef = useRef<MonacoDisposable | null>(null);
  const [fallback, setFallback] = useState(false);

  useEffect(() => {
    let disposed = false;
    async function mountMonaco() {
      if (!containerRef.current || typeof window === "undefined") {
        setFallback(true);
        return;
      }
      try {
        const loaded = await import("monaco-editor/esm/vs/editor/editor.api");
        if (disposed || !containerRef.current) return;
        monacoRef.current = loaded;
        editorRef.current = loaded.editor.create(containerRef.current, {
          value,
          language,
          readOnly: readonly,
          automaticLayout: true,
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          fontSize: 12,
          tabSize: 2,
          wordWrap: "on",
          lineNumbersMinChars: 3,
          padding: { top: 10, bottom: 10 },
          theme: "vs-dark",
        });
        listenerRef.current = editorRef.current.onDidChangeModelContent(() => {
          const next = editorRef.current?.getValue() || "";
          if (next !== value) onChange?.(next);
        });
      } catch {
        setFallback(true);
      }
    }
    void mountMonaco();
    return () => {
      disposed = true;
      listenerRef.current?.dispose();
      editorRef.current?.dispose();
      listenerRef.current = null;
      editorRef.current = null;
    };
  }, []);

  useEffect(() => {
    const editor = editorRef.current;
    if (editor && editor.getValue() !== value) {
      editor.setValue(value);
    }
  }, [value]);

  useEffect(() => {
    editorRef.current?.updateOptions({ readOnly: readonly });
  }, [readonly]);

  useEffect(() => {
    const model = editorRef.current?.getModel();
    if (model && monacoRef.current) {
      monacoRef.current.editor.setModelLanguage(model, language);
    }
  }, [language]);

  if (fallback) {
    return (
      <textarea
        className="code-editor-fallback"
        value={value}
        readOnly={readonly}
        placeholder={placeholder}
        style={{ height }}
        onChange={(event) => onChange?.(event.target.value)}
      />
    );
  }

  return <div ref={containerRef} className="code-editor" style={{ height }} />;
}
