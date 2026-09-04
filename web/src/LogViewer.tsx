import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { cn, SegmentedControl, ToolDialog } from "./ui";

// LogViewer is the live-tailing log dialog shared by the panel and Xray log views.
// Each caller supplies the SSE url, title, filter tabs, a `classify` fn mapping a
// line to a category, and `colorOf` mapping a category to a text-color class.
export function LogViewer({
  title,
  streamUrl,
  onClose,
  filters,
  classify,
  colorOf,
}: {
  title: string;
  streamUrl: string;
  onClose: () => void;
  filters: { value: string; label: string }[];
  classify: (line: string) => string;
  colorOf: (cat: string) => string;
}) {
  const { t } = useTranslation();
  const [lines, setLines] = useState<string[]>([]);
  const [level, setLevel] = useState("all");
  const [atBottom, setAtBottom] = useState(true);
  const [reconnecting, setReconnecting] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);

  useEffect(() => {
    let es: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let attempt = 0;
    let unmounted = false;

    const connect = () => {
      if (unmounted) return;
      try {
        es = new EventSource(streamUrl, { withCredentials: true });
      } catch {
        handleError();
        return;
      }

      es.onopen = () => {
        attempt = 0;
        setReconnecting(false);
      };

      es.onmessage = (e) => {
        setReconnecting(false);
        setLines((prev) => {
          const next = [...prev, e.data];
          return next.length > 2000 ? next.slice(-2000) : next;
        });
      };

      es.onerror = () => {
        handleError();
      };
    };

    const handleError = () => {
      if (unmounted) return;
      if (es) {
        es.close();
        es = null;
      }
      attempt++;
      setReconnecting(true);
      const delay = Math.min(2000 * Math.pow(1.5, attempt - 1), 10000);
      timer = setTimeout(connect, delay);
    };

    connect();

    return () => {
      unmounted = true;
      if (es) es.close();
      if (timer) clearTimeout(timer);
    };
  }, [streamUrl]);

  const shown =
    level === "all" ? lines : lines.filter((l) => classify(l) === level);

  // Auto-scroll to the bottom unless the user scrolled up to read history.
  useEffect(() => {
    if (stick.current && boxRef.current) {
      boxRef.current.scrollTop = boxRef.current.scrollHeight;
    }
  }, [shown.length]);

  const onScroll = () => {
    const el = boxRef.current;
    if (!el) return;
    const bottom = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
    stick.current = bottom;
    setAtBottom(bottom);
  };

  const scrollToBottom = () => {
    const el = boxRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    stick.current = true;
    setAtBottom(true);
  };

  return (
    <ToolDialog
      title={title}
      onClose={onClose}
      headerExtra={
        <SegmentedControl data={filters} value={level} onChange={setLevel} />
      }
    >
      {reconnecting && (
        <div className="flex items-center gap-2 border-b border-amber-200 bg-amber-50 px-3 py-1.5 text-xs text-amber-800">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-amber-500" />
          <span>{t("logs.reconnecting", "Соединение прервано. Переподключение...")}</span>
        </div>
      )}
      <div
        ref={boxRef}
        onScroll={onScroll}
        className="flex-1 overflow-auto bg-gray-50 p-3 font-mono text-xs leading-relaxed"
      >
        {shown.length === 0 ? (
          <p className="text-gray-400">
            {lines.length === 0
              ? t("logs.waiting")
              : t("logs.noLinesAtLevel")}
          </p>
        ) : (
          shown.map((l, i) => (
            <div
              key={i}
              className={cn("whitespace-pre-wrap break-all", colorOf(classify(l)))}
            >
              {l}
            </div>
          ))
        )}
      </div>
      {!atBottom && (
        <button
          onClick={scrollToBottom}
          aria-label={t("logs.scrollDown")}
          className="absolute bottom-4 right-4 z-20 flex h-10 w-10 items-center justify-center rounded-full bg-brand-600 text-onaccent shadow-lg transition hover:bg-brand-700"
        >
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M12 5v14M5 12l7 7 7-7" />
          </svg>
        </button>
      )}
    </ToolDialog>
  );
}
