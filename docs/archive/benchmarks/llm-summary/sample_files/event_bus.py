"""
Event bus implementation for decoupled component communication.

This module provides a publish-subscribe event system for loose coupling
between components in a Python application.
"""

import asyncio
import logging
import weakref
from collections import defaultdict
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum, auto
from typing import (
    Any,
    Awaitable,
    Callable,
    Dict,
    Generic,
    List,
    Optional,
    Set,
    TypeVar,
    Union,
)

logger = logging.getLogger(__name__)

T = TypeVar("T")


class EventPriority(Enum):
    """Priority levels for event handlers."""

    LOW = auto()
    NORMAL = auto()
    HIGH = auto()
    CRITICAL = auto()


@dataclass
class Event(Generic[T]):
    """Base event class with metadata."""

    name: str
    payload: T
    timestamp: datetime = field(default_factory=datetime.utcnow)
    source: Optional[str] = None
    correlation_id: Optional[str] = None
    metadata: Dict[str, Any] = field(default_factory=dict)

    def with_metadata(self, **kwargs: Any) -> "Event[T]":
        """Create a copy with additional metadata."""
        new_metadata = {**self.metadata, **kwargs}
        return Event(
            name=self.name,
            payload=self.payload,
            timestamp=self.timestamp,
            source=self.source,
            correlation_id=self.correlation_id,
            metadata=new_metadata,
        )


# Type aliases for handlers
SyncHandler = Callable[[Event[Any]], None]
AsyncHandler = Callable[[Event[Any]], Awaitable[None]]
Handler = Union[SyncHandler, AsyncHandler]


@dataclass
class Subscription:
    """Represents an event subscription."""

    handler: Handler
    priority: EventPriority = EventPriority.NORMAL
    once: bool = False
    filter_fn: Optional[Callable[[Event[Any]], bool]] = None

    def matches(self, event: Event[Any]) -> bool:
        """Check if subscription matches the event."""
        if self.filter_fn is None:
            return True
        try:
            return self.filter_fn(event)
        except Exception as e:
            logger.warning(f"Filter function failed: {e}")
            return False


class EventBus:
    """
    Central event bus for publish-subscribe messaging.

    Supports both synchronous and asynchronous handlers,
    with priority ordering and optional filtering.
    """

    def __init__(self, max_history: int = 100):
        self._subscriptions: Dict[str, List[Subscription]] = defaultdict(list)
        self._history: List[Event[Any]] = []
        self._max_history = max_history
        self._stats: Dict[str, int] = defaultdict(int)
        self._paused: Set[str] = set()
        self._lock = asyncio.Lock()

    def subscribe(
        self,
        event_name: str,
        handler: Handler,
        priority: EventPriority = EventPriority.NORMAL,
        once: bool = False,
        filter_fn: Optional[Callable[[Event[Any]], bool]] = None,
    ) -> Callable[[], None]:
        """
        Subscribe to an event.

        Args:
            event_name: Name of the event to subscribe to
            handler: Callback function (sync or async)
            priority: Handler priority (higher runs first)
            once: If True, handler is removed after first call
            filter_fn: Optional filter function

        Returns:
            Unsubscribe function
        """
        subscription = Subscription(
            handler=handler,
            priority=priority,
            once=once,
            filter_fn=filter_fn,
        )

        # Insert maintaining priority order (highest first)
        subs = self._subscriptions[event_name]
        insert_idx = 0
        for i, sub in enumerate(subs):
            if sub.priority.value < priority.value:
                insert_idx = i
                break
            insert_idx = i + 1

        subs.insert(insert_idx, subscription)
        logger.debug(f"Subscribed to '{event_name}' with priority {priority.name}")

        # Return unsubscribe function
        def unsubscribe() -> None:
            try:
                self._subscriptions[event_name].remove(subscription)
                logger.debug(f"Unsubscribed from '{event_name}'")
            except ValueError:
                pass

        return unsubscribe

    def on(
        self,
        event_name: str,
        priority: EventPriority = EventPriority.NORMAL,
    ) -> Callable[[Handler], Handler]:
        """
        Decorator for subscribing to events.

        @bus.on('user.created')
        async def handle_user_created(event):
            print(f"User created: {event.payload}")
        """

        def decorator(handler: Handler) -> Handler:
            self.subscribe(event_name, handler, priority=priority)
            return handler

        return decorator

    async def publish(self, event: Event[Any]) -> int:
        """
        Publish an event to all subscribers.

        Args:
            event: The event to publish

        Returns:
            Number of handlers that processed the event
        """
        if event.name in self._paused:
            logger.debug(f"Event '{event.name}' is paused, skipping")
            return 0

        # Record in history
        self._history.append(event)
        if len(self._history) > self._max_history:
            self._history = self._history[-self._max_history :]

        # Update stats
        self._stats[event.name] += 1
        self._stats["total"] += 1

        handlers_called = 0
        to_remove: List[Subscription] = []

        for subscription in self._subscriptions.get(event.name, []):
            if not subscription.matches(event):
                continue

            try:
                if asyncio.iscoroutinefunction(subscription.handler):
                    await subscription.handler(event)
                else:
                    subscription.handler(event)
                handlers_called += 1

                if subscription.once:
                    to_remove.append(subscription)

            except Exception as e:
                logger.error(f"Handler error for '{event.name}': {e}")
                self._stats["errors"] += 1

        # Remove one-time handlers
        for sub in to_remove:
            try:
                self._subscriptions[event.name].remove(sub)
            except ValueError:
                pass

        logger.debug(f"Published '{event.name}' to {handlers_called} handlers")
        return handlers_called

    def emit(self, name: str, payload: Any, **metadata: Any) -> asyncio.Task[int]:
        """
        Convenience method to create and publish an event.

        Returns an asyncio Task that resolves to the handler count.
        """
        event = Event(name=name, payload=payload, metadata=metadata)
        return asyncio.create_task(self.publish(event))

    def pause(self, event_name: str) -> None:
        """Pause processing of a specific event."""
        self._paused.add(event_name)
        logger.info(f"Paused event: {event_name}")

    def resume(self, event_name: str) -> None:
        """Resume processing of a paused event."""
        self._paused.discard(event_name)
        logger.info(f"Resumed event: {event_name}")

    def clear(self, event_name: Optional[str] = None) -> None:
        """Clear subscriptions for an event or all events."""
        if event_name:
            self._subscriptions[event_name].clear()
        else:
            self._subscriptions.clear()

    def get_history(
        self, event_name: Optional[str] = None, limit: int = 10
    ) -> List[Event[Any]]:
        """Get recent event history."""
        history = self._history
        if event_name:
            history = [e for e in history if e.name == event_name]
        return history[-limit:]

    @property
    def stats(self) -> Dict[str, int]:
        """Get event statistics."""
        return dict(self._stats)

    def subscriber_count(self, event_name: str) -> int:
        """Get number of subscribers for an event."""
        return len(self._subscriptions.get(event_name, []))


# Global event bus instance
_default_bus: Optional[EventBus] = None


def get_event_bus() -> EventBus:
    """Get or create the default event bus."""
    global _default_bus
    if _default_bus is None:
        _default_bus = EventBus()
    return _default_bus


def publish(name: str, payload: Any, **metadata: Any) -> asyncio.Task[int]:
    """Publish an event to the default bus."""
    return get_event_bus().emit(name, payload, **metadata)


def subscribe(
    event_name: str,
    handler: Handler,
    priority: EventPriority = EventPriority.NORMAL,
) -> Callable[[], None]:
    """Subscribe to an event on the default bus."""
    return get_event_bus().subscribe(event_name, handler, priority=priority)
