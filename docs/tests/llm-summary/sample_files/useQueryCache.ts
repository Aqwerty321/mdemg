/**
 * React hook for managing cached API queries with automatic refetching.
 *
 * This hook provides a simple interface for data fetching with:
 * - Automatic caching based on query keys
 * - Background refetching on window focus
 * - Stale-while-revalidate pattern
 * - Error retry with exponential backoff
 */

import { useState, useEffect, useCallback, useRef } from 'react';

export interface QueryConfig<T> {
  /** Unique key for caching */
  queryKey: string | string[];
  /** Function that fetches the data */
  queryFn: () => Promise<T>;
  /** Time in ms before data is considered stale (default: 5 minutes) */
  staleTime?: number;
  /** Time in ms to keep data in cache (default: 30 minutes) */
  cacheTime?: number;
  /** Whether to refetch on window focus (default: true) */
  refetchOnWindowFocus?: boolean;
  /** Number of retry attempts on error (default: 3) */
  retryCount?: number;
  /** Initial retry delay in ms (default: 1000) */
  retryDelay?: number;
  /** Whether query is enabled (default: true) */
  enabled?: boolean;
  /** Callback on successful fetch */
  onSuccess?: (data: T) => void;
  /** Callback on error */
  onError?: (error: Error) => void;
}

export interface QueryResult<T> {
  data: T | undefined;
  isLoading: boolean;
  isFetching: boolean;
  isError: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
  invalidate: () => void;
}

interface CacheEntry<T> {
  data: T;
  timestamp: number;
  staleTime: number;
}

// Global cache store
const queryCache = new Map<string, CacheEntry<unknown>>();

/**
 * Custom hook for data fetching with caching and automatic refetching.
 *
 * @example
 * ```tsx
 * const { data, isLoading, error } = useQueryCache({
 *   queryKey: ['users', userId],
 *   queryFn: () => fetchUser(userId),
 *   staleTime: 60000, // 1 minute
 * });
 * ```
 */
export function useQueryCache<T>(config: QueryConfig<T>): QueryResult<T> {
  const {
    queryKey,
    queryFn,
    staleTime = 5 * 60 * 1000, // 5 minutes
    cacheTime = 30 * 60 * 1000, // 30 minutes
    refetchOnWindowFocus = true,
    retryCount = 3,
    retryDelay = 1000,
    enabled = true,
    onSuccess,
    onError,
  } = config;

  const cacheKey = Array.isArray(queryKey) ? queryKey.join(':') : queryKey;

  const [data, setData] = useState<T | undefined>(() => {
    const cached = queryCache.get(cacheKey) as CacheEntry<T> | undefined;
    return cached?.data;
  });
  const [isLoading, setIsLoading] = useState<boolean>(!data && enabled);
  const [isFetching, setIsFetching] = useState<boolean>(false);
  const [error, setError] = useState<Error | null>(null);

  const retryCountRef = useRef(0);
  const abortControllerRef = useRef<AbortController | null>(null);

  const isStale = useCallback((): boolean => {
    const cached = queryCache.get(cacheKey) as CacheEntry<T> | undefined;
    if (!cached) return true;
    return Date.now() - cached.timestamp > cached.staleTime;
  }, [cacheKey]);

  const fetchData = useCallback(
    async (showLoading: boolean = true): Promise<void> => {
      if (!enabled) return;

      // Cancel any in-flight request
      abortControllerRef.current?.abort();
      abortControllerRef.current = new AbortController();

      if (showLoading && !data) {
        setIsLoading(true);
      }
      setIsFetching(true);
      setError(null);

      try {
        const result = await queryFn();

        // Update cache
        queryCache.set(cacheKey, {
          data: result,
          timestamp: Date.now(),
          staleTime,
        });

        setData(result);
        retryCountRef.current = 0;
        onSuccess?.(result);

        // Schedule cache cleanup
        setTimeout(() => {
          const cached = queryCache.get(cacheKey);
          if (cached && Date.now() - cached.timestamp > cacheTime) {
            queryCache.delete(cacheKey);
          }
        }, cacheTime);
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));

        // Retry logic with exponential backoff
        if (retryCountRef.current < retryCount) {
          retryCountRef.current++;
          const delay = retryDelay * Math.pow(2, retryCountRef.current - 1);
          setTimeout(() => fetchData(false), delay);
          return;
        }

        setError(error);
        onError?.(error);
      } finally {
        setIsLoading(false);
        setIsFetching(false);
      }
    },
    [
      enabled,
      data,
      queryFn,
      cacheKey,
      staleTime,
      cacheTime,
      retryCount,
      retryDelay,
      onSuccess,
      onError,
    ]
  );

  const refetch = useCallback(async (): Promise<void> => {
    retryCountRef.current = 0;
    await fetchData(true);
  }, [fetchData]);

  const invalidate = useCallback((): void => {
    queryCache.delete(cacheKey);
    setData(undefined);
    retryCountRef.current = 0;
    fetchData(true);
  }, [cacheKey, fetchData]);

  // Initial fetch
  useEffect(() => {
    if (enabled && (isStale() || !data)) {
      fetchData(!data);
    }
  }, [enabled, isStale, data, fetchData]);

  // Refetch on window focus
  useEffect(() => {
    if (!refetchOnWindowFocus || !enabled) return;

    const handleFocus = () => {
      if (isStale()) {
        fetchData(false);
      }
    };

    window.addEventListener('focus', handleFocus);
    return () => window.removeEventListener('focus', handleFocus);
  }, [refetchOnWindowFocus, enabled, isStale, fetchData]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort();
    };
  }, []);

  return {
    data,
    isLoading,
    isFetching,
    isError: error !== null,
    error,
    refetch,
    invalidate,
  };
}

/**
 * Invalidate all queries matching a key prefix.
 */
export function invalidateQueries(keyPrefix: string): void {
  for (const key of queryCache.keys()) {
    if (key.startsWith(keyPrefix)) {
      queryCache.delete(key);
    }
  }
}

/**
 * Clear the entire query cache.
 */
export function clearQueryCache(): void {
  queryCache.clear();
}

/**
 * Prefetch data and store in cache.
 */
export async function prefetchQuery<T>(
  queryKey: string | string[],
  queryFn: () => Promise<T>,
  staleTime: number = 5 * 60 * 1000
): Promise<void> {
  const cacheKey = Array.isArray(queryKey) ? queryKey.join(':') : queryKey;

  try {
    const data = await queryFn();
    queryCache.set(cacheKey, {
      data,
      timestamp: Date.now(),
      staleTime,
    });
  } catch (error) {
    console.error('Prefetch failed:', error);
  }
}
