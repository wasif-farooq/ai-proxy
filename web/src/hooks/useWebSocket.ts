import { useEffect, useRef, useState, useCallback } from 'react';

interface UseWebSocketOptions {
  /** WebSocket URL to connect to. If relative, uses the current origin. */
  url: string;
  /** Called with parsed JSON message data on each incoming message. */
  onMessage: (data: unknown) => void;
  /** Whether to connect on mount. Defaults to true. */
  autoConnect?: boolean;
  /** Maximum reconnect attempts before giving up. Defaults to Infinity. */
  maxRetries?: number;
}

interface UseWebSocketReturn {
  /** Whether the WebSocket is currently connected. */
  connected: boolean;
  /** Number of reconnect attempts made. */
  retryCount: number;
  /** Manually reconnect. */
  reconnect: () => void;
  /** Manually disconnect. */
  disconnect: () => void;
}

/**
 * useWebSocket — connects to a WebSocket endpoint and delivers parsed JSON
 * messages to the onMessage callback. Handles reconnection with exponential
 * backoff and cleans up on unmount.
 */
export const useWebSocket = ({
  url,
  onMessage,
  autoConnect = true,
  maxRetries = Infinity,
}: UseWebSocketOptions): UseWebSocketReturn => {
  const [connected, setConnected] = useState(false);
  const [retryCount, setRetryCount] = useState(0);

  const wsRef = useRef<WebSocket | null>(null);
  const onMessageRef = useRef(onMessage);
  const retryTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);
  const retryCountRef = useRef(0);

  // Keep the callback ref current without re-triggering the effect
  onMessageRef.current = onMessage;

  const disconnect = useCallback(() => {
    if (retryTimeoutRef.current) {
      clearTimeout(retryTimeoutRef.current);
      retryTimeoutRef.current = null;
    }
    if (wsRef.current) {
      wsRef.current.onclose = null; // prevent reconnect
      wsRef.current.onerror = null;
      wsRef.current.close();
      wsRef.current = null;
    }
    setConnected(false);
  }, []);

  const connect = useCallback(() => {
    if (!mountedRef.current) return;
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    // Resolve relative URLs against the current origin, replacing http→ws
    let wsUrl = url;
    if (wsUrl.startsWith('/')) {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      wsUrl = `${protocol}//${window.location.host}${wsUrl}`;
    }

    try {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        if (!mountedRef.current) {
          ws.close();
          return;
        }
        setConnected(true);
        setRetryCount(0);
        retryCountRef.current = 0;
      };

      ws.onmessage = (event: MessageEvent) => {
        try {
          const data = JSON.parse(event.data);
          onMessageRef.current(data);
        } catch {
          // Ignore non-JSON messages (e.g., ping/pong frames)
        }
      };

      ws.onclose = () => {
        setConnected(false);
        wsRef.current = null;

        // Auto-reconnect with exponential backoff
        if (mountedRef.current && retryCountRef.current < maxRetries) {
          const delay = Math.min(1000 * 2 ** retryCountRef.current, 30_000);
          retryTimeoutRef.current = setTimeout(() => {
            retryCountRef.current++;
            setRetryCount(retryCountRef.current);
            connect();
          }, delay);
        }
      };

      ws.onerror = () => {
        // onclose will fire after onerror, triggering reconnect
        ws.close();
      };
    } catch {
      // WebSocket constructor failed — retry
      if (mountedRef.current && retryCountRef.current < maxRetries) {
        const delay = Math.min(1000 * 2 ** retryCountRef.current, 30_000);
        retryTimeoutRef.current = setTimeout(() => {
          retryCountRef.current++;
          setRetryCount(retryCountRef.current);
          connect();
        }, delay);
      }
    }
  }, [url, maxRetries]);

  const reconnect = useCallback(() => {
    disconnect();
    connect();
  }, [disconnect, connect]);

  useEffect(() => {
    mountedRef.current = true;

    if (autoConnect) {
      connect();
    }

    return () => {
      mountedRef.current = false;
      disconnect();
    };
  }, [autoConnect, connect, disconnect]);

  return { connected, retryCount, reconnect, disconnect };
};
