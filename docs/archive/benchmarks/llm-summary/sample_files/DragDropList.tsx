/**
 * DragDropList - A reorderable list component using dnd-kit.
 *
 * This component provides drag-and-drop reordering functionality
 * with keyboard accessibility, smooth animations, and touch support.
 */

import React, { useState, useCallback } from 'react';
import {
  DndContext,
  DragEndEvent,
  DragStartEvent,
  DragOverlay,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
  arrayMove,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

export interface DragDropItem {
  id: string;
  content: React.ReactNode;
  disabled?: boolean;
}

export interface DragDropListProps<T extends DragDropItem> {
  /** Array of items to render */
  items: T[];
  /** Callback when items are reordered */
  onReorder: (items: T[]) => void;
  /** Custom render function for each item */
  renderItem?: (item: T, isDragging: boolean) => React.ReactNode;
  /** CSS class for the list container */
  className?: string;
  /** CSS class for each item */
  itemClassName?: string;
  /** Whether drag and drop is disabled */
  disabled?: boolean;
  /** Callback when drag starts */
  onDragStart?: (item: T) => void;
  /** Callback when drag ends */
  onDragEnd?: (item: T, newIndex: number) => void;
}

interface SortableItemProps {
  id: string;
  children: React.ReactNode;
  className?: string;
  disabled?: boolean;
}

/**
 * Individual sortable item wrapper component.
 */
function SortableItem({ id, children, className, disabled }: SortableItemProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id, disabled });

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    cursor: disabled ? 'default' : 'grab',
    touchAction: 'none',
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={className}
      {...attributes}
      {...listeners}
      role="listitem"
      aria-grabbed={isDragging}
    >
      {children}
    </div>
  );
}

/**
 * DragDropList component for reorderable lists.
 *
 * @example
 * ```tsx
 * const [items, setItems] = useState([
 *   { id: '1', content: 'Item 1' },
 *   { id: '2', content: 'Item 2' },
 *   { id: '3', content: 'Item 3' },
 * ]);
 *
 * <DragDropList
 *   items={items}
 *   onReorder={setItems}
 *   renderItem={(item) => <div>{item.content}</div>}
 * />
 * ```
 */
export function DragDropList<T extends DragDropItem>({
  items,
  onReorder,
  renderItem,
  className = '',
  itemClassName = '',
  disabled = false,
  onDragStart,
  onDragEnd,
}: DragDropListProps<T>) {
  const [activeId, setActiveId] = useState<string | null>(null);

  // Configure sensors for different input methods
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8, // Minimum drag distance to start
      },
    }),
    useSensor(TouchSensor, {
      activationConstraint: {
        delay: 250, // Long press delay for touch
        tolerance: 5,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  const handleDragStart = useCallback(
    (event: DragStartEvent) => {
      const { active } = event;
      setActiveId(active.id as string);

      if (onDragStart) {
        const item = items.find((i) => i.id === active.id);
        if (item) onDragStart(item);
      }
    },
    [items, onDragStart]
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      setActiveId(null);

      if (over && active.id !== over.id) {
        const oldIndex = items.findIndex((item) => item.id === active.id);
        const newIndex = items.findIndex((item) => item.id === over.id);

        const newItems = arrayMove(items, oldIndex, newIndex);
        onReorder(newItems);

        if (onDragEnd) {
          const item = items[oldIndex];
          onDragEnd(item, newIndex);
        }
      }
    },
    [items, onReorder, onDragEnd]
  );

  const activeItem = activeId ? items.find((item) => item.id === activeId) : null;

  const defaultRenderItem = (item: T, isDragging: boolean) => (
    <div
      style={{
        padding: '12px 16px',
        backgroundColor: isDragging ? '#f0f0f0' : '#ffffff',
        border: '1px solid #e0e0e0',
        borderRadius: '4px',
        marginBottom: '8px',
      }}
    >
      {item.content}
    </div>
  );

  const itemRenderer = renderItem || defaultRenderItem;

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
    >
      <SortableContext
        items={items.map((item) => item.id)}
        strategy={verticalListSortingStrategy}
        disabled={disabled}
      >
        <div className={className} role="list" aria-label="Reorderable list">
          {items.map((item) => (
            <SortableItem
              key={item.id}
              id={item.id}
              className={itemClassName}
              disabled={disabled || item.disabled}
            >
              {itemRenderer(item, item.id === activeId)}
            </SortableItem>
          ))}
        </div>
      </SortableContext>

      {/* Drag overlay for smooth visual feedback */}
      <DragOverlay dropAnimation={null}>
        {activeItem ? (
          <div
            style={{
              transform: 'scale(1.02)',
              boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
            }}
          >
            {itemRenderer(activeItem, true)}
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}

/**
 * Hook for programmatic control of drag and drop.
 */
export function useDragDropControl<T extends DragDropItem>(
  items: T[],
  onReorder: (items: T[]) => void
) {
  const moveItem = useCallback(
    (fromIndex: number, toIndex: number) => {
      if (fromIndex < 0 || fromIndex >= items.length) return;
      if (toIndex < 0 || toIndex >= items.length) return;

      const newItems = arrayMove(items, fromIndex, toIndex);
      onReorder(newItems);
    },
    [items, onReorder]
  );

  const moveToTop = useCallback(
    (index: number) => moveItem(index, 0),
    [moveItem]
  );

  const moveToBottom = useCallback(
    (index: number) => moveItem(index, items.length - 1),
    [moveItem, items.length]
  );

  const moveUp = useCallback(
    (index: number) => moveItem(index, Math.max(0, index - 1)),
    [moveItem]
  );

  const moveDown = useCallback(
    (index: number) => moveItem(index, Math.min(items.length - 1, index + 1)),
    [moveItem, items.length]
  );

  return {
    moveItem,
    moveToTop,
    moveToBottom,
    moveUp,
    moveDown,
  };
}
