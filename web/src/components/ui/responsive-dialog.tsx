import * as React from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "~/components/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "~/components/ui/drawer";
import { useIsMobile } from "~/lib/use-is-mobile";

interface ResponsiveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: React.ReactNode;
  /** Optional supporting text under the title. */
  description?: React.ReactNode;
  /** Extra classes for the desktop dialog panel (e.g. `sm:max-w-md`). */
  contentClassName?: string;
  /**
   * Override the default open-focus behavior. Radix/vaul auto-focus the
   * first focusable element on open; for a dialog whose first field is a
   * native `<input type="date">`, that auto-focus makes mobile browsers pop
   * the OS date picker the instant the dialog appears. Pass
   * `(e) => e.preventDefault()` to suppress it.
   */
  onOpenAutoFocus?: (event: Event) => void;
  children: React.ReactNode;
}

/**
 * A modal that renders as a swipe-up bottom sheet on mobile (vaul Drawer) and a
 * centered dialog on desktop (Radix Dialog), sharing the same body. This is the
 * standard pattern for every form popup in the app — see issue #20.
 *
 * Controlled only (`open` / `onOpenChange`); confirmations (AlertDialog) and the
 * desktop-only Header menu intentionally do not use this.
 */
export function ResponsiveDialog({
  open,
  onOpenChange,
  title,
  description,
  contentClassName,
  onOpenAutoFocus,
  children,
}: ResponsiveDialogProps) {
  const isMobile = useIsMobile();

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent onOpenAutoFocus={onOpenAutoFocus}>
          <DrawerHeader>
            <DrawerTitle className="text-[var(--sea-ink)]">{title}</DrawerTitle>
            {description ? (
              <DrawerDescription>{description}</DrawerDescription>
            ) : null}
          </DrawerHeader>
          <div className="flex flex-1 flex-col gap-4 overflow-y-auto overflow-x-hidden px-4 pb-6">
            {children}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={contentClassName} onOpenAutoFocus={onOpenAutoFocus}>
        <DialogHeader>
          <DialogTitle className="text-[var(--sea-ink)]">{title}</DialogTitle>
          {description ? (
            <DialogDescription>{description}</DialogDescription>
          ) : null}
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}
