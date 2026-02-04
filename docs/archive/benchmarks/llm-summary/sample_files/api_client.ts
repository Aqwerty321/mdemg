/**
 * API Client - Type-safe HTTP client with interceptors and retry logic.
 *
 * Provides a unified interface for making HTTP requests with:
 * - Automatic request/response transformation
 * - Request/response interceptors
 * - Retry with exponential backoff
 * - Request deduplication
 * - Timeout handling
 */

export interface RequestConfig {
  baseURL?: string;
  headers?: Record<string, string>;
  timeout?: number;
  retries?: number;
  retryDelay?: number;
  withCredentials?: boolean;
}

export interface RequestOptions extends RequestInit {
  params?: Record<string, string | number | boolean>;
  data?: unknown;
  timeout?: number;
  retries?: number;
}

export interface ApiResponse<T> {
  data: T;
  status: number;
  statusText: string;
  headers: Headers;
  config: RequestOptions;
}

export interface ApiError extends Error {
  status?: number;
  statusText?: string;
  data?: unknown;
  config?: RequestOptions;
}

type Interceptor<T> = (value: T) => T | Promise<T>;

interface InterceptorManager<T> {
  use(interceptor: Interceptor<T>): number;
  eject(id: number): void;
  forEach(fn: (interceptor: Interceptor<T>) => void): void;
}

/**
 * Create an interceptor manager for request or response interceptors.
 */
function createInterceptorManager<T>(): InterceptorManager<T> {
  const interceptors: Map<number, Interceptor<T>> = new Map();
  let id = 0;

  return {
    use(interceptor: Interceptor<T>): number {
      interceptors.set(++id, interceptor);
      return id;
    },
    eject(interceptorId: number): void {
      interceptors.delete(interceptorId);
    },
    forEach(fn: (interceptor: Interceptor<T>) => void): void {
      interceptors.forEach((interceptor) => fn(interceptor));
    },
  };
}

/**
 * Build URL with query parameters.
 */
function buildURL(
  baseURL: string,
  path: string,
  params?: Record<string, string | number | boolean>
): string {
  const url = new URL(path, baseURL);

  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null) {
        url.searchParams.append(key, String(value));
      }
    });
  }

  return url.toString();
}

/**
 * Sleep for a specified duration.
 */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * API Client class for making HTTP requests.
 */
export class ApiClient {
  private config: Required<RequestConfig>;
  private pendingRequests: Map<string, Promise<ApiResponse<unknown>>> = new Map();

  public interceptors = {
    request: createInterceptorManager<RequestOptions>(),
    response: createInterceptorManager<ApiResponse<unknown>>(),
  };

  constructor(config: RequestConfig = {}) {
    this.config = {
      baseURL: config.baseURL || '',
      headers: config.headers || {},
      timeout: config.timeout || 30000,
      retries: config.retries || 0,
      retryDelay: config.retryDelay || 1000,
      withCredentials: config.withCredentials || false,
    };
  }

  /**
   * Make an HTTP request.
   */
  async request<T>(
    method: string,
    path: string,
    options: RequestOptions = {}
  ): Promise<ApiResponse<T>> {
    // Apply request interceptors
    let requestOptions: RequestOptions = {
      method,
      headers: { ...this.config.headers, ...options.headers },
      credentials: this.config.withCredentials ? 'include' : 'same-origin',
      ...options,
    };

    await this.runInterceptors(this.interceptors.request, requestOptions);

    // Build URL
    const url = buildURL(this.config.baseURL, path, requestOptions.params);

    // Serialize body
    if (requestOptions.data) {
      requestOptions.body = JSON.stringify(requestOptions.data);
      requestOptions.headers = {
        'Content-Type': 'application/json',
        ...requestOptions.headers,
      };
    }

    // Check for duplicate request
    const requestKey = `${method}:${url}`;
    const pending = this.pendingRequests.get(requestKey);
    if (pending && method === 'GET') {
      return pending as Promise<ApiResponse<T>>;
    }

    // Create request promise
    const requestPromise = this.executeRequest<T>(url, requestOptions);

    // Store pending request for deduplication (GET only)
    if (method === 'GET') {
      this.pendingRequests.set(requestKey, requestPromise);
      requestPromise.finally(() => {
        this.pendingRequests.delete(requestKey);
      });
    }

    return requestPromise;
  }

  /**
   * Execute the actual HTTP request with retry logic.
   */
  private async executeRequest<T>(
    url: string,
    options: RequestOptions
  ): Promise<ApiResponse<T>> {
    const maxRetries = options.retries ?? this.config.retries;
    const timeout = options.timeout ?? this.config.timeout;
    let lastError: ApiError | null = null;

    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      try {
        // Create abort controller for timeout
        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), timeout);

        const response = await fetch(url, {
          ...options,
          signal: controller.signal,
        });

        clearTimeout(timeoutId);

        // Parse response
        let data: T;
        const contentType = response.headers.get('content-type');
        if (contentType?.includes('application/json')) {
          data = await response.json();
        } else {
          data = (await response.text()) as unknown as T;
        }

        // Check for error status
        if (!response.ok) {
          const error: ApiError = new Error(response.statusText);
          error.status = response.status;
          error.statusText = response.statusText;
          error.data = data;
          error.config = options;
          throw error;
        }

        // Build response
        let apiResponse: ApiResponse<T> = {
          data,
          status: response.status,
          statusText: response.statusText,
          headers: response.headers,
          config: options,
        };

        // Apply response interceptors
        await this.runInterceptors(
          this.interceptors.response,
          apiResponse as ApiResponse<unknown>
        );

        return apiResponse;
      } catch (error) {
        lastError = error as ApiError;

        // Don't retry on client errors (4xx)
        if (lastError.status && lastError.status >= 400 && lastError.status < 500) {
          throw lastError;
        }

        // Retry with exponential backoff
        if (attempt < maxRetries) {
          const delay = this.config.retryDelay * Math.pow(2, attempt);
          await sleep(delay);
        }
      }
    }

    throw lastError;
  }

  /**
   * Run interceptors in sequence.
   */
  private async runInterceptors<T>(
    manager: InterceptorManager<T>,
    value: T
  ): Promise<T> {
    let result = value;
    manager.forEach(async (interceptor) => {
      result = await interceptor(result);
    });
    return result;
  }

  // Convenience methods
  get<T>(path: string, options?: RequestOptions): Promise<ApiResponse<T>> {
    return this.request<T>('GET', path, options);
  }

  post<T>(path: string, data?: unknown, options?: RequestOptions): Promise<ApiResponse<T>> {
    return this.request<T>('POST', path, { ...options, data });
  }

  put<T>(path: string, data?: unknown, options?: RequestOptions): Promise<ApiResponse<T>> {
    return this.request<T>('PUT', path, { ...options, data });
  }

  patch<T>(path: string, data?: unknown, options?: RequestOptions): Promise<ApiResponse<T>> {
    return this.request<T>('PATCH', path, { ...options, data });
  }

  delete<T>(path: string, options?: RequestOptions): Promise<ApiResponse<T>> {
    return this.request<T>('DELETE', path, options);
  }
}

/**
 * Create a new API client instance.
 */
export function createApiClient(config?: RequestConfig): ApiClient {
  return new ApiClient(config);
}

// Default instance
export const apiClient = createApiClient({
  baseURL: process.env.NEXT_PUBLIC_API_URL || '/api',
  timeout: 30000,
  retries: 2,
});
