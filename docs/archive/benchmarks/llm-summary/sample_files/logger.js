/**
 * Structured logging utility with multiple transports.
 *
 * Provides consistent logging across the application with support for:
 * - Multiple log levels (debug, info, warn, error)
 * - Structured JSON output
 * - Multiple transports (console, file, external services)
 * - Context propagation
 * - Performance timing
 */

const LOG_LEVELS = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

/**
 * Format a log entry as JSON.
 */
function formatJSON(entry) {
  return JSON.stringify({
    timestamp: entry.timestamp,
    level: entry.level,
    message: entry.message,
    context: entry.context,
    ...(entry.error && {
      error: {
        name: entry.error.name,
        message: entry.error.message,
        stack: entry.error.stack,
      },
    }),
    ...(entry.duration !== undefined && { duration_ms: entry.duration }),
    ...(entry.metadata && { metadata: entry.metadata }),
  });
}

/**
 * Format a log entry for console output.
 */
function formatConsole(entry) {
  const timestamp = new Date(entry.timestamp).toISOString();
  const level = entry.level.toUpperCase().padEnd(5);
  const context = entry.context ? `[${entry.context}] ` : '';
  const duration = entry.duration !== undefined ? ` (${entry.duration}ms)` : '';

  let message = `${timestamp} ${level} ${context}${entry.message}${duration}`;

  if (entry.metadata && Object.keys(entry.metadata).length > 0) {
    message += ` ${JSON.stringify(entry.metadata)}`;
  }

  return message;
}

/**
 * Console transport for logging.
 */
class ConsoleTransport {
  constructor(options = {}) {
    this.minLevel = options.minLevel || 'debug';
    this.colorize = options.colorize !== false;
    this.format = options.format || 'console'; // 'console' or 'json'
  }

  log(entry) {
    if (LOG_LEVELS[entry.level] < LOG_LEVELS[this.minLevel]) {
      return;
    }

    const formatted =
      this.format === 'json' ? formatJSON(entry) : formatConsole(entry);

    const colors = {
      debug: '\x1b[36m', // cyan
      info: '\x1b[32m', // green
      warn: '\x1b[33m', // yellow
      error: '\x1b[31m', // red
    };
    const reset = '\x1b[0m';

    if (this.colorize && this.format === 'console') {
      console[entry.level](`${colors[entry.level]}${formatted}${reset}`);
    } else {
      console[entry.level](formatted);
    }

    if (entry.error && entry.level === 'error') {
      console.error(entry.error);
    }
  }
}

/**
 * HTTP transport for sending logs to external service.
 */
class HttpTransport {
  constructor(options) {
    this.endpoint = options.endpoint;
    this.headers = options.headers || {};
    this.minLevel = options.minLevel || 'info';
    this.batchSize = options.batchSize || 10;
    this.flushInterval = options.flushInterval || 5000;
    this.buffer = [];

    // Set up periodic flush
    if (typeof setInterval !== 'undefined') {
      setInterval(() => this.flush(), this.flushInterval);
    }
  }

  log(entry) {
    if (LOG_LEVELS[entry.level] < LOG_LEVELS[this.minLevel]) {
      return;
    }

    this.buffer.push(entry);

    if (this.buffer.length >= this.batchSize) {
      this.flush();
    }
  }

  async flush() {
    if (this.buffer.length === 0) return;

    const entries = this.buffer;
    this.buffer = [];

    try {
      await fetch(this.endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...this.headers,
        },
        body: JSON.stringify({ logs: entries }),
      });
    } catch (error) {
      // Re-add entries to buffer on failure (with limit)
      this.buffer = [...entries.slice(-50), ...this.buffer].slice(-100);
      console.error('Failed to send logs:', error);
    }
  }
}

/**
 * Logger class with support for multiple transports.
 */
class Logger {
  constructor(options = {}) {
    this.context = options.context || null;
    this.transports = options.transports || [new ConsoleTransport()];
    this.defaultMetadata = options.metadata || {};
  }

  /**
   * Create a child logger with additional context.
   */
  child(context, metadata = {}) {
    return new Logger({
      context: this.context ? `${this.context}:${context}` : context,
      transports: this.transports,
      metadata: { ...this.defaultMetadata, ...metadata },
    });
  }

  /**
   * Log a message at the specified level.
   */
  log(level, message, metadata = {}) {
    const entry = {
      timestamp: Date.now(),
      level,
      message,
      context: this.context,
      metadata: { ...this.defaultMetadata, ...metadata },
    };

    // Extract error if present
    if (metadata instanceof Error) {
      entry.error = metadata;
      entry.metadata = this.defaultMetadata;
    } else if (metadata.error instanceof Error) {
      entry.error = metadata.error;
      delete entry.metadata.error;
    }

    for (const transport of this.transports) {
      try {
        transport.log(entry);
      } catch (error) {
        console.error('Transport error:', error);
      }
    }
  }

  debug(message, metadata) {
    this.log('debug', message, metadata);
  }

  info(message, metadata) {
    this.log('info', message, metadata);
  }

  warn(message, metadata) {
    this.log('warn', message, metadata);
  }

  error(message, metadata) {
    this.log('error', message, metadata);
  }

  /**
   * Time an async operation.
   */
  async time(label, fn) {
    const start = Date.now();
    try {
      const result = await fn();
      const duration = Date.now() - start;
      this.info(`${label} completed`, { duration });
      return result;
    } catch (error) {
      const duration = Date.now() - start;
      this.error(`${label} failed`, { duration, error });
      throw error;
    }
  }

  /**
   * Create a timer for manual duration tracking.
   */
  startTimer(label) {
    const start = Date.now();
    return {
      done: (metadata = {}) => {
        const duration = Date.now() - start;
        this.info(`${label} completed`, { ...metadata, duration });
      },
      error: (error, metadata = {}) => {
        const duration = Date.now() - start;
        this.error(`${label} failed`, { ...metadata, duration, error });
      },
    };
  }
}

/**
 * Create the default logger instance.
 */
function createLogger(options = {}) {
  const transports = [];

  // Console transport (always enabled)
  transports.push(
    new ConsoleTransport({
      minLevel: options.minLevel || process.env.LOG_LEVEL || 'info',
      colorize: process.env.NODE_ENV !== 'production',
      format: process.env.LOG_FORMAT || 'console',
    })
  );

  // HTTP transport (if endpoint configured)
  if (options.httpEndpoint || process.env.LOG_ENDPOINT) {
    transports.push(
      new HttpTransport({
        endpoint: options.httpEndpoint || process.env.LOG_ENDPOINT,
        minLevel: 'warn',
        headers: options.httpHeaders || {},
      })
    );
  }

  return new Logger({
    context: options.context,
    transports,
    metadata: options.metadata,
  });
}

// Export default logger and factory
const logger = createLogger();

module.exports = {
  Logger,
  ConsoleTransport,
  HttpTransport,
  createLogger,
  logger,
  LOG_LEVELS,
};
