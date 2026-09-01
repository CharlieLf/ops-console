import { useCallback, useEffect, useRef, useState } from "react";

interface AsyncState<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  reload: () => void;
}

/**
 * Runs an async loader on mount and optionally on an interval. The loader is
 * held in a ref so callers can pass an inline arrow without re-subscribing.
 */
export function useAsync<T>(loader: () => Promise<T>, intervalMs = 0): AsyncState<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const loaderRef = useRef(loader);
  loaderRef.current = loader;
  const alive = useRef(true);

  const run = useCallback(async () => {
    try {
      const next = await loaderRef.current();
      if (!alive.current) return;
      setData(next);
      setError(null);
    } catch (err) {
      if (!alive.current) return;
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (alive.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void run();
    if (intervalMs <= 0) return () => { alive.current = false; };
    const id = window.setInterval(run, intervalMs);
    return () => {
      alive.current = false;
      window.clearInterval(id);
    };
  }, [run, intervalMs]);

  return { data, error, loading, reload: run };
}
