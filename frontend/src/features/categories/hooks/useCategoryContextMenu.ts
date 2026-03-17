import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";

import type { CategoryLookups, ParentKey } from "../types";

export interface ContextMenuState {
  open: boolean;
  nodeId: number | null;
  parentId: ParentKey;
  x: number;
  y: number;
}

interface OpenContextMenuPayload {
  nodeId: number;
  parentId: ParentKey;
  x: number;
  y: number;
}

interface UseCategoryContextMenuParams {
  menuDebugEnabled: boolean;
  lookups: CategoryLookups;
}

export function useCategoryContextMenu({
  menuDebugEnabled,
  lookups,
}: UseCategoryContextMenuParams) {
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({
    open: false,
    nodeId: null,
    parentId: null,
    x: 0,
    y: 0,
  });
  const menuContainerRef = useRef<HTMLDivElement | null>(null);

  const closeContextMenu = useCallback(
    (reason?: string) => {
      setContextMenu((prev) => {
        if (!prev.open) {
          if (menuDebugEnabled) {
            // eslint-disable-next-line no-console
            console.log("[menu-debug] skip close (already closed)", { reason, previous: prev });
          }
          return prev;
        }
        const next = { ...prev, open: false };
        if (menuDebugEnabled) {
          // eslint-disable-next-line no-console
          console.log("[menu-debug] closing context menu", { reason, previous: prev, next });
        }
        return next;
      });
    },
    [menuDebugEnabled],
  );

  const openContextMenu = useCallback(
    ({ nodeId, parentId, x, y }: OpenContextMenuPayload) => {
      const nextState: ContextMenuState = {
        open: true,
        nodeId,
        parentId,
        x,
        y,
      };
      if (menuDebugEnabled) {
        // eslint-disable-next-line no-console
        console.log("[menu-debug] opening context menu", { state: nextState });
      }
      setContextMenu(nextState);
    },
    [menuDebugEnabled],
  );

  useEffect(() => {
    if (!contextMenu.open) {
      return;
    }
    if (contextMenu.nodeId == null) {
      closeContextMenu("missing-node-id");
      return;
    }
    if (!lookups.byId.has(contextMenu.nodeId)) {
      closeContextMenu("stale-node-reference");
    }
  }, [closeContextMenu, contextMenu, lookups.byId]);

  useEffect(() => {
    if (!contextMenu.open) {
      return;
    }
    const handlePointerDown = (event: MouseEvent | PointerEvent) => {
      if ("button" in event && event.button === 2) {
        if (menuDebugEnabled) {
          // eslint-disable-next-line no-console
          console.log("[menu-debug] pointerdown ignored (right button)", {
            type: event.type,
            button: event.button,
          });
        }
        return;
      }
      const container = menuContainerRef.current;
      if (container && event.target instanceof Node && container.contains(event.target)) {
        if (menuDebugEnabled) {
          // eslint-disable-next-line no-console
          console.log("[menu-debug] pointerdown inside menu, skip close", {
            type: event.type,
          });
        }
        return;
      }
      if (menuDebugEnabled) {
        // eslint-disable-next-line no-console
        console.log("[menu-debug] pointerdown closing context menu", {
          type: event.type,
          button: "button" in event ? event.button : null,
        });
      }
      closeContextMenu(`pointerdown:${event.type}`);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        if (menuDebugEnabled) {
          // eslint-disable-next-line no-console
          console.log("[menu-debug] escape pressed, closing menu");
        }
        closeContextMenu("escape-key");
      }
    };
    document.addEventListener("pointerdown", handlePointerDown, true);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown, true);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [closeContextMenu, contextMenu.open, menuDebugEnabled]);

  const suppressNativeContextMenu = useCallback(
    (event: ReactMouseEvent<Element>) => {
      if (menuDebugEnabled) {
        const target = event.target as HTMLElement | null;
        const targetInfo =
          target instanceof HTMLElement
            ? {
                tag: target.tagName,
                className: target.className,
                dataset: { ...target.dataset },
              }
            : null;
        // eslint-disable-next-line no-console
        console.log("[menu-debug] suppress native contextmenu", {
          type: event.nativeEvent?.type ?? event.type,
          target: targetInfo,
        });
      }
      event.preventDefault();
      if (typeof event.nativeEvent?.preventDefault === "function") {
        event.nativeEvent.preventDefault();
      }
    },
    [menuDebugEnabled],
  );

  // 菜单打开后测量实际尺寸，动态调整坐标确保不超出视口
  const [adjustedPosition, setAdjustedPosition] = useState<{ x: number; y: number }>({ x: 0, y: 0 });

  useLayoutEffect(() => {
    if (!contextMenu.open) return;
    const el = menuContainerRef.current;
    if (!el) {
      setAdjustedPosition({ x: contextMenu.x, y: contextMenu.y });
      return;
    }
    const rect = el.getBoundingClientRect();
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let top = contextMenu.y;
    let left = contextMenu.x;
    if (top + rect.height > vh) top = vh - rect.height - 8;
    if (left + rect.width > vw) left = vw - rect.width - 8;
    if (top < 0) top = 8;
    if (left < 0) left = 8;
    setAdjustedPosition({ x: left, y: top });
  }, [contextMenu.open, contextMenu.x, contextMenu.y]);

  return {
    contextMenu,
    adjustedPosition,
    openContextMenu,
    closeContextMenu,
    suppressNativeContextMenu,
    menuContainerRef,
  } as const;
}
