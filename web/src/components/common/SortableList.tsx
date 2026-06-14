import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import type { ReactNode } from 'react';

/* ─── Sortable item ────────────────────────────────────── */

interface SortableItemProps {
  id: string;
  children: ReactNode;
  onRemove?: (id: string) => void;
}

function SortableItem({ id, children, onRemove }: SortableItemProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.4 : 1,
    zIndex: isDragging ? 10 : 'auto' as const,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`
        flex items-center gap-2 px-3 py-2 rounded-xs border border-hairline
        bg-surface-card ${isDragging ? 'shadow-md' : ''}
        transition-shadow duration-150
      `}
    >
      {/* Drag handle */}
      <button
        type="button"
        className="flex items-center justify-center w-6 h-6 rounded-xs text-muted-soft hover:text-ink hover:bg-surface-strong cursor-grab active:cursor-grabbing transition-colors duration-100 shrink-0 border-none"
        aria-label="Drag to reorder"
        {...attributes}
        {...listeners}
      >
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
          <circle cx="4" cy="2" r="1" fill="currentColor" />
          <circle cx="8" cy="2" r="1" fill="currentColor" />
          <circle cx="4" cy="6" r="1" fill="currentColor" />
          <circle cx="8" cy="6" r="1" fill="currentColor" />
          <circle cx="4" cy="10" r="1" fill="currentColor" />
          <circle cx="8" cy="10" r="1" fill="currentColor" />
        </svg>
      </button>

      {/* Content */}
      <div className="flex-1 min-w-0">{children}</div>

      {/* Remove button */}
      {onRemove && (
        <button
          type="button"
          onClick={() => onRemove(id)}
          className="flex items-center justify-center w-6 h-6 rounded-xs text-muted-soft hover:text-primary-error hover:bg-surface-strong transition-colors duration-100 shrink-0 cursor-pointer border-none"
          aria-label={`Remove ${id}`}
        >
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
            <path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      )}
    </div>
  );
}

/* ─── Sortable list root ────────────────────────────────── */

interface SortableListProps<T extends { id: string }> {
  items: T[];
  onChange: (items: T[]) => void;
  renderItem: (item: T) => ReactNode;
  onRemove?: (id: string) => void;
  className?: string;
}

export function SortableList<T extends { id: string }>({
  items,
  onChange,
  renderItem,
  onRemove,
  className = '',
}: SortableListProps<T>) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = items.findIndex((i) => i.id === active.id);
    const newIndex = items.findIndex((i) => i.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;

    onChange(arrayMove(items, oldIndex, newIndex));
  };

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={items.map((i) => i.id)} strategy={verticalListSortingStrategy}>
        <div className={`flex flex-col gap-1.5 ${className}`}>
          {items.map((item) => (
            <SortableItem key={item.id} id={item.id} onRemove={onRemove}>
              {renderItem(item)}
            </SortableItem>
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}

/* ─── Empty state ──────────────────────────────────────── */

export function SortableEmpty({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-center justify-center px-4 py-6 border border-dashed border-hairline rounded-xs bg-surface-soft/30 text-muted-soft text-[13px] font-[400] leading-[1.23]">
      {children}
    </div>
  );
}
