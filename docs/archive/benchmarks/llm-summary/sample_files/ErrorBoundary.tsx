/**
 * ErrorBoundary - React error boundary with retry and reporting.
 *
 * Catches JavaScript errors in child component tree and displays
 * a fallback UI while optionally reporting errors to a tracking service.
 */

import React, { Component, ErrorInfo, ReactNode } from 'react';

export interface ErrorBoundaryProps {
  /** Child components to wrap */
  children: ReactNode;
  /** Custom fallback UI to show on error */
  fallback?: ReactNode | ((error: Error, retry: () => void) => ReactNode);
  /** Callback when error is caught */
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
  /** Error reporting service URL */
  reportingEndpoint?: string;
  /** Whether to show error details in development */
  showErrorDetails?: boolean;
  /** Maximum retry attempts */
  maxRetries?: number;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
  retryCount: number;
}

/**
 * Error boundary component for graceful error handling in React apps.
 *
 * @example
 * ```tsx
 * <ErrorBoundary
 *   fallback={<ErrorPage />}
 *   onError={(error) => logError(error)}
 * >
 *   <App />
 * </ErrorBoundary>
 * ```
 */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  static defaultProps = {
    showErrorDetails: process.env.NODE_ENV === 'development',
    maxRetries: 3,
  };

  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
      retryCount: 0,
    };
  }

  /**
   * Update state when an error is caught during rendering.
   */
  static getDerivedStateFromError(error: Error): Partial<ErrorBoundaryState> {
    return {
      hasError: true,
      error,
    };
  }

  /**
   * Log error details and optionally report to external service.
   */
  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    this.setState({ errorInfo });

    // Log to console in development
    if (process.env.NODE_ENV === 'development') {
      console.error('ErrorBoundary caught an error:', error);
      console.error('Component stack:', errorInfo.componentStack);
    }

    // Call custom error handler
    this.props.onError?.(error, errorInfo);

    // Report to external service
    this.reportError(error, errorInfo);
  }

  /**
   * Send error report to external tracking service.
   */
  private async reportError(error: Error, errorInfo: ErrorInfo): Promise<void> {
    const { reportingEndpoint } = this.props;
    if (!reportingEndpoint) return;

    try {
      await fetch(reportingEndpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          name: error.name,
          message: error.message,
          stack: error.stack,
          componentStack: errorInfo.componentStack,
          url: window.location.href,
          userAgent: navigator.userAgent,
          timestamp: new Date().toISOString(),
        }),
      });
    } catch (reportError) {
      // Silently fail - don't throw during error handling
      console.warn('Failed to report error:', reportError);
    }
  }

  /**
   * Reset error state and attempt to re-render children.
   */
  private handleRetry = (): void => {
    const { maxRetries = 3 } = this.props;

    if (this.state.retryCount < maxRetries) {
      this.setState((prevState) => ({
        hasError: false,
        error: null,
        errorInfo: null,
        retryCount: prevState.retryCount + 1,
      }));
    }
  };

  /**
   * Reset the error boundary completely.
   */
  public reset(): void {
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
      retryCount: 0,
    });
  }

  render(): ReactNode {
    const { hasError, error, errorInfo, retryCount } = this.state;
    const { children, fallback, showErrorDetails, maxRetries = 3 } = this.props;

    if (hasError && error) {
      // Render custom fallback if provided as function
      if (typeof fallback === 'function') {
        return fallback(error, this.handleRetry);
      }

      // Render custom fallback component
      if (fallback) {
        return fallback;
      }

      // Default error UI
      return (
        <div
          role="alert"
          style={{
            padding: '20px',
            margin: '20px',
            borderRadius: '8px',
            backgroundColor: '#fef2f2',
            border: '1px solid #fecaca',
          }}
        >
          <h2 style={{ color: '#dc2626', marginTop: 0 }}>
            Something went wrong
          </h2>

          <p style={{ color: '#7f1d1d' }}>
            An unexpected error occurred. Please try again.
          </p>

          {retryCount < maxRetries && (
            <button
              onClick={this.handleRetry}
              style={{
                padding: '8px 16px',
                backgroundColor: '#dc2626',
                color: 'white',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer',
                marginRight: '10px',
              }}
            >
              Try Again ({maxRetries - retryCount} attempts left)
            </button>
          )}

          <button
            onClick={() => window.location.reload()}
            style={{
              padding: '8px 16px',
              backgroundColor: '#6b7280',
              color: 'white',
              border: 'none',
              borderRadius: '4px',
              cursor: 'pointer',
            }}
          >
            Reload Page
          </button>

          {showErrorDetails && (
            <details style={{ marginTop: '20px' }}>
              <summary style={{ cursor: 'pointer', color: '#7f1d1d' }}>
                Error Details
              </summary>
              <pre
                style={{
                  marginTop: '10px',
                  padding: '10px',
                  backgroundColor: '#1f2937',
                  color: '#f3f4f6',
                  borderRadius: '4px',
                  overflow: 'auto',
                  fontSize: '12px',
                }}
              >
                {error.toString()}
                {errorInfo?.componentStack}
              </pre>
            </details>
          )}
        </div>
      );
    }

    return children;
  }
}

/**
 * Higher-order component to wrap a component with ErrorBoundary.
 */
export function withErrorBoundary<P extends object>(
  WrappedComponent: React.ComponentType<P>,
  errorBoundaryProps?: Omit<ErrorBoundaryProps, 'children'>
): React.FC<P> {
  const displayName =
    WrappedComponent.displayName || WrappedComponent.name || 'Component';

  const WithErrorBoundary: React.FC<P> = (props) => (
    <ErrorBoundary {...errorBoundaryProps}>
      <WrappedComponent {...props} />
    </ErrorBoundary>
  );

  WithErrorBoundary.displayName = `withErrorBoundary(${displayName})`;
  return WithErrorBoundary;
}

/**
 * Hook to trigger error boundary from event handlers or effects.
 */
export function useErrorHandler(): (error: Error) => void {
  const [, setError] = React.useState<Error>();

  return React.useCallback((error: Error) => {
    setError(() => {
      throw error;
    });
  }, []);
}
